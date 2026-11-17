// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: MIT

package ui

import "testing"

func TestObserveWindowPointerHoverCaptureAndUnmount(t *testing.T) {
	show, disabled := true, false
	var seen []InputEvent
	tt := NewTester(func(c *Context) {
		if show {
			Box(c).Absolute().Left(20).Top(30).Size(80, 80).Disabled(disabled).ObserveWindowPointer(func(ev InputEvent) { seen = append(seen, ev) })
		}
		Box(c).Absolute().Left(200).Top(100).Size(50, 50).HandleInput(func(ev InputEvent) bool {
			return ev.Kind == InputPointerDown || ev.Kind == InputPointerMove || ev.Kind == InputPointerUp
		})
	}, 400, 300)
	tt.Move(350, 250)
	if len(seen) != 1 || seen[0].X != 330 || seen[0].Y != 220 || seen[0].Button != -1 || seen[0].Pointer != PointerMouse {
		t.Fatal("outside hover", seen)
	}
	seen = nil
	tt.Press(210, 110)
	tt.Move(250, 150)
	if len(seen) != 2 || seen[1].Button != 0 || seen[1].Pressure != .5 {
		t.Fatal("captured movement", seen)
	}
	tt.Release(250, 150)
	seen = nil
	disabled = true
	tt.Frame()
	tt.Move(40, 50)
	if len(seen) != 0 {
		t.Fatal("disabled observer received input")
	}
	disabled = false
	show = false
	tt.Frame()
	tt.Move(60, 70)
	if len(seen) != 0 || len(tt.rt.windowPointerObservers) != 0 {
		t.Fatal("unmounted callback retained")
	}
}

func TestObserveWindowPointerDoesNotConsumeOrRequestFrame(t *testing.T) {
	calls := 0
	tt := NewTester(func(c *Context) { Box(c).Size(80, 80).ObserveWindowPointer(func(InputEvent) { calls++ }) }, 300, 200)
	tt.Frame()
	tt.h.requested.Store(false)
	tt.rt.pointerMove(250, 150)
	if calls != 1 || tt.h.requested.Load() {
		t.Fatal("passive callback requested a native rebuild", calls, tt.h.requested.Load())
	}
}

func TestWebViewPointerEventsPassesPointerAndKeepsFocus(t *testing.T) {
	v := &testWebView{"passive"}
	tt := NewTester(func(c *Context) { WebView(c, v).Size(120, 90).WebViewPointerEvents(false) }, 200, 160)
	if _, ok := placed(tt, v); !ok {
		t.Fatal("passive page was hidden")
	}
	if tt.WebViewAt(30, 30) != nil {
		t.Fatal("ignored web view captured pointer")
	}
	if len(tt.rt.focusOrder) != 1 {
		t.Fatal("pointer-events:none unexpectedly removed keyboard focus")
	}
}
