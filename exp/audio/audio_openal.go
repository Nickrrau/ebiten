// Copyright 2015 Hajime Hoshi
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build !js,!windows

package audio

import (
	"fmt"
	"io"
	"runtime"
	"sync"
	"time"

	"golang.org/x/mobile/exp/audio/al"
)

// alSourceCacheEntry represents a source.
// In some environments, too many calls of alGenSources and alGenBuffers could cause errors.
// To avoid this error, al.Source and al.Buffer are reused.
type alSourceCacheEntry struct {
	source     al.Source
	buffers    []al.Buffer
	sampleRate int
	isClosed   bool
}

const maxSourceNum = 32

var alSourceCache = []*alSourceCacheEntry{}

type player struct {
	alSource   al.Source
	alBuffers  []al.Buffer
	source     io.ReadSeeker
	sampleRate int
}

var m sync.Mutex

func newAlSource(sampleRate int) (al.Source, []al.Buffer, error) {
	for _, e := range alSourceCache {
		if e.sampleRate != sampleRate {
			continue
		}
		if !e.isClosed {
			continue
		}
		e.isClosed = false
		return e.source, e.buffers, nil
	}
	if maxSourceNum <= len(alSourceCache) {
		return 0, nil, ErrTooManyPlayers
	}
	s := al.GenSources(1)
	if err := al.Error(); err != 0 {
		panic(fmt.Sprintf("audio: al.GenSources error: %d", err))
	}
	e := &alSourceCacheEntry{
		source:     s[0],
		buffers:    []al.Buffer{},
		sampleRate: sampleRate,
	}
	alSourceCache = append(alSourceCache, e)
	return s[0], e.buffers, nil
}

func newPlayer(src io.ReadSeeker, sampleRate int) (*Player, error) {
	m.Lock()
	if e := al.OpenDevice(); e != nil {
		m.Unlock()
		return nil, fmt.Errorf("audio: OpenAL initialization failed: %v", e)
	}
	s, b, err := newAlSource(sampleRate)
	if err != nil {
		m.Unlock()
		return nil, err
	}
	m.Unlock()
	p := &player{
		alSource:   s,
		alBuffers:  b,
		source:     src,
		sampleRate: sampleRate,
	}
	runtime.SetFinalizer(p, (*player).close)
	return &Player{p}, nil
}

const bufferSize = 1024

func (p *player) proceed() error {
	m.Lock()
	if err := al.Error(); err != 0 {
		panic(fmt.Sprintf("audio: before proceed: %d", err))
	}
	processedNum := p.alSource.BuffersProcessed()
	if 0 < processedNum {
		bufs := make([]al.Buffer, processedNum)
		p.alSource.UnqueueBuffers(bufs...)
		if err := al.Error(); err != 0 {
			panic(fmt.Sprintf("audio: Unqueue in process: %d", err))
		}
		p.alBuffers = append(p.alBuffers, bufs...)
	}
	m.Unlock()
	for 0 < len(p.alBuffers) {
		b := make([]byte, bufferSize)
		n, err := p.source.Read(b)
		if 0 < n {
			m.Lock()
			buf := p.alBuffers[0]
			p.alBuffers = p.alBuffers[1:]
			buf.BufferData(al.FormatStereo16, b[:n], int32(p.sampleRate))
			p.alSource.QueueBuffers(buf)
			if err := al.Error(); err != 0 {
				panic(fmt.Sprintf("audio: Queue in process: %d", err))
			}
			m.Unlock()
		}
		if err != nil {
			return err
		}
	}
	m.Lock()
	if p.alSource.State() == al.Stopped {
		al.RewindSources(p.alSource)
		al.PlaySources(p.alSource)
		if err := al.Error(); err != 0 {
			panic(fmt.Sprintf("audio: PlaySource in process: %d", err))
		}
	}
	m.Unlock()

	return nil
}

func (p *player) play() error {
	const bufferMaxNum = 8
	// TODO: What if play is already called?
	m.Lock()
	n := bufferMaxNum - int(p.alSource.BuffersQueued()) - len(p.alBuffers)
	if 0 < n {
		p.alBuffers = append(p.alBuffers, al.GenBuffers(n)...)
	}
	if 0 < len(p.alBuffers) {
		emptyBytes := make([]byte, bufferSize)
		for _, buf := range p.alBuffers {
			// Note that the third argument of only the first buffer is used.
			buf.BufferData(al.FormatStereo16, emptyBytes, int32(p.sampleRate))
			p.alSource.QueueBuffers(buf)
		}
		p.alBuffers = []al.Buffer{}
	}
	al.PlaySources(p.alSource)
	m.Unlock()
	go func() {
		defer p.close()
		for {
			err := p.proceed()
			if err == io.EOF {
				break
			}
			if err != nil {
				// TODO: Record the last error
				panic(err)
			}
			time.Sleep(1)
		}
	}()
	return nil
}

func (p *player) close() error {
	m.Lock()
	if err := al.Error(); err != 0 {
		panic(fmt.Sprintf("audio: error before closing: %d", err))
	}
	s := p.alSource
	if p.alSource != 0 {
		var bs []al.Buffer
		al.RewindSources(p.alSource)
		al.StopSources(p.alSource)
		n := p.alSource.BuffersQueued()
		if 0 < n {
			bs = make([]al.Buffer, n)
			p.alSource.UnqueueBuffers(bs...)
			p.alBuffers = append(p.alBuffers, bs...)
		}
		p.alSource = 0
	}
	if err := al.Error(); err != 0 {
		panic(fmt.Sprintf("audio: closing error: %d", err))
	}
	if s != 0 {
		found := false
		for _, e := range alSourceCache {
			if e.source != s {
				continue
			}
			if e.isClosed {
				panic("audio: cache state is invalid: source is already closed?")
			}
			e.buffers = p.alBuffers
			e.isClosed = true
			found = true
			break
		}
		if !found {
			panic("audio: cache state is invalid: source is not cached?")
		}
	}
	m.Unlock()
	runtime.SetFinalizer(p, nil)
	return nil
}