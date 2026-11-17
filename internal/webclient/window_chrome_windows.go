//go:build windows

// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"syscall"

	"github.com/egoist/mygo"
)

var (
	chromeUser32         = syscall.NewLazyDLL("user32.dll")
	chromeReleaseCapture = chromeUser32.NewProc("ReleaseCapture")
	chromeSendMessage    = chromeUser32.NewProc("SendMessageW")
)

func startWindowDrag(window *mygo.Window) {
	mygo.RunOnMain(func() {
		if window.IsDestroyed() {
			return
		}
		chromeReleaseCapture.Call()
		// Enter Windows' normal caption move loop, retaining drag-to-snap and
		// maximized-window restoration across monitors and DPI boundaries.
		chromeSendMessage.Call(window.NativeHandle(), 0x00a1, 2, 0)
	})
}
