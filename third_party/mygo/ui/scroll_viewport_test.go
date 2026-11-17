package ui

import "testing"

func TestScrollViewportNearestBorderAndTransform(t *testing.T) {
	var outer, inner, leaf, outside *node
	var scroll ScrollState
	tt := coreNewTester(func(c *context) {
		outside = coreBox(c).Size(1, 1)
		outer = coreScroll(c).Size(300, 240)
		outer.Children(func() {
			coreBox(c).Height(25).Shrink(0)
			inner = coreScroll(c).Size(200, 100).Border(3, Hex("000000")).Translate(7, 9).TrackScroll(&scroll)
			inner.Children(func() { leaf = coreBox(c).Size(100, 500).Shrink(0) })
			coreBox(c).Height(400).Shrink(0)
		})
	}, 500, 500)
	tt.Frame()
	if r, ok := outside.ScrollViewport(); ok || r != (Rect{}) {
		t.Fatalf("outside: %v %v", r, ok)
	}
	if r, ok := outer.ScrollViewport(); ok || r != (Rect{}) {
		t.Fatalf("outer has no ancestor: %v %v", r, ok)
	}
	want := inner.Bounds()
	want.X += 3
	want.Y += 3
	want.W -= 6
	want.H -= 6
	if r, ok := leaf.ScrollViewport(); !ok || r != want {
		t.Fatalf("nearest transformed viewport: got %v %v want %v", r, ok, want)
	}
	if r, ok := inner.ScrollViewport(); !ok || r != outer.Bounds() {
		t.Fatalf("outer viewport: %v %v", r, ok)
	}
	scroll.Y = 80
	tt.Frame()
	if r, ok := leaf.ScrollViewport(); !ok || r != want {
		t.Fatalf("content scroll moved viewport: %v %v", r, ok)
	}
}
