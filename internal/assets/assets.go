/*
Copyright 2014 Hajime Hoshi

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package assets

import (
	"bytes"
	"image"
	_ "image/png"
)

const FileNameText = "text.png"

//go:generate go-bindata -nocompress -pkg=assets -nomemcopy text.png

const (
	TextImageWidth      = 192
	TextImageHeight     = 128
	TextImageCharWidth  = TextImageWidth / 32
	TextImageCharHeight = TextImageHeight / 8
)

func TextImage() (image.Image, error) {
	b, err := Asset("text.png")
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewBuffer(b))
	return img, err
}
