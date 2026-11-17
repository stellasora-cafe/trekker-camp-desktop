// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"github.com/egoist/mygo/ui"
	"html/template"
)

const windowCaptionWidth = 42

// Both the native caption controls and the built-in WebView use these assets.
func captionAsset(name string) []byte {
	data, err := aboutFiles.ReadFile("about/chrome/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return data
}
func captionAssetHTML(name string) template.HTML { return template.HTML(captionAsset(name)) }

var campMinimizeIcon = ui.MustParseSVG(captionAsset("minimize"))
var campMaximizeIcon = ui.MustParseSVG(captionAsset("maximize"))
var campRestoreIcon = ui.MustParseSVG(captionAsset("restore"))
var campCloseIcon = ui.MustParseSVG(captionAsset("close"))
