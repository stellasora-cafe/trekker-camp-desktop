// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: MIT

package ui

import (
	"slices"
	"testing"
)

func TestObserveInputTracksPassiveDragOutsideRegion(t *testing.T) {
	var seen []InputEvent
	tt := NewTester(func(c *Context) {
		Box(c).Absolute().Left(20).Top(30).Size(100, 100).ObserveInput(func(ev InputEvent) { seen = append(seen, ev) })
	}, 400, 300)
	tt.Frame()
	tt.Move(40, 50)
	if len(seen) != 0 {
		t.Fatal("unpressed movement was observed", seen)
	}
	tt.PressButton(2, 40, 50)
	tt.Move(250, 230)
	tt.ReleaseButton(2, 250, 230)
	if len(seen) != 3 {
		t.Fatalf("passive drag lost or repeated events: %+v", seen)
	}
	kinds := []InputKind{InputPointerDown, InputPointerMove, InputPointerUp}
	for i, ev := range seen {
		if ev.Kind != kinds[i] || ev.Button != 2 || ev.Pointer != PointerMouse {
			t.Fatal(i, ev)
		}
		if i == 0 && (ev.X != 20 || ev.Y != 20 || ev.Pressure != .5) {
			t.Fatal("down coordinates/device", ev)
		}
		if i > 0 && (ev.X != 230 || ev.Y != 200) {
			t.Fatal("outside local coordinates", ev)
		}
	}
	if seen[1].Pressure != .5 || seen[2].Pressure != 0 {
		t.Fatal("drag pressure", seen)
	}
	seen = nil
	tt.Move(40, 50)
	if len(seen) != 0 {
		t.Fatal("released chain retained")
	}
}

func TestObserveInputDragCaptureOrderAndTouchCancel(t *testing.T) {
	var log []string
	var seen []InputEvent
	tt := NewTester(func(c *Context) {
		Box(c).Size(120, 120).ObserveInput(func(ev InputEvent) { log = append(log, "outer"); seen = append(seen, ev) }).Children(func() {
			Box(c).Size(80, 80).ObserveInput(func(ev InputEvent) { log = append(log, "inner") }).HandleInput(func(ev InputEvent) bool {
				if ev.Kind == InputPointerDown || ev.Kind == InputPointerMove || ev.Kind == InputPointerUp {
					log = append(log, "handler")
					return true
				}
				return false
			})
		})
	}, 400, 300)
	tt.Frame()
	tt.Move(20, 20)
	log = nil
	tt.UsePointer(PointerTouch, 0)
	tt.Press(20, 20)
	tt.Move(250, 230)
	tt.CancelPress()
	want := []string{"outer", "inner", "handler", "outer", "inner", "handler", "outer", "inner", "handler"}
	if !slices.Equal(log, want) {
		t.Fatal("capture duplicated or reordered observation", log)
	}
	if len(seen) != 3 {
		t.Fatal(seen)
	}
	for _, ev := range seen {
		if ev.Pointer != PointerTouch {
			t.Fatal("lost touch device", ev)
		}
	}
	if !seen[2].Canceled || seen[2].Pressure != 0 {
		t.Fatal("canceled press not released", seen)
	}
	log, seen = nil, nil
	tt.Move(240, 220)
	if len(log) != 0 || len(seen) != 0 {
		t.Fatal("canceled chain kept observing", log, seen)
	}
}

func TestObserveInputDragSkipsUnmountedObserver(t *testing.T) {
	show := true
	var count int
	tt := NewTester(func(c *Context) {
		if show {
			Box(c).Size(100, 100).ObserveInput(func(ev InputEvent) { count++ })
		}
	}, 300, 300)
	tt.Frame()
	tt.PressButton(2, 20, 20)
	show = false
	tt.Frame()
	tt.Move(200, 200)
	tt.ReleaseButton(2, 200, 200)
	if count != 1 {
		t.Fatal("unmounted observer received late input", count)
	}
}
