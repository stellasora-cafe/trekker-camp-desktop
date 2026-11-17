package ui

import (
	"image/color"
	"testing"
	"time"
)

func TestViewTransitionRetainsPaintAndControls(t *testing.T) {
	value, next := RGB(255, 0, 0), RGB(0, 0, 255)
	requested, changes := false, 0
	text := "draft"
	cv := NewCanvas(20, 20)
	tt := coreNewTester(func(c *context) {
		if requested {
			requested = false
			makeContext(c).StartViewTransition(500*time.Millisecond, Linear, func() { value = next; changes++ })
		}
		coreColumn(c).Fill().Children(func() {
			coreTextInput(c, &text).Label("draft")
			coreBox(c).Size(80, 80).Draw(func(p *Painter, r Rect) {
				cv.Draw(p, func(cp *Painter) { cp.Fill(Rect{W: 20, H: 20}, value, 0) })
				p.Image(cv.Bitmap(), r, FillBox)
			})
		})
	}, 120, 130)
	if err := tt.Click("draft"); err != nil {
		t.Fatal(err)
	}
	tt.Type(" kept")
	before := text
	requested = true
	tt.Frame()
	if changes != 1 || !tt.Focused("draft") || text != before {
		t.Fatalf("update=%d focus=%v draft=%q", changes, tt.Focused("draft"), text)
	}
	if got := tt.Image().RGBAAt(10, 80); got != (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("old frame = %v", got)
	}
	transition := tt.rt.viewTransition
	if transition.canvas.img.Scene.MaskAtlas == tt.rt.scene.MaskAtlas {
		t.Fatal("captured text still shares the live glyph atlas")
	}
	// CapturePage starts a CPU renderer after the GPU has drawn the snapshot.
	// Discard the software caches to exercise that first read after the live
	// canvas already changed from red to blue.
	for _, img := range transition.frozen {
		img.Pix, img.PixVersion = nil, 0
	}
	tt.h.img.Render(&tt.rt.scene)
	if got := tt.Image().RGBAAt(10, 80); got != (color.RGBA{255, 0, 0, 255}) {
		t.Fatalf("capture repainted a changed nested canvas: %v", got)
	}
	tt.Advance(250 * time.Millisecond)
	got := tt.Image().RGBAAt(10, 80)
	if got.R < 120 || got.R > 140 || got.B < 115 || got.B > 135 || got.G != 0 {
		t.Fatalf("half fade = %v", got)
	}
	tt.Advance(300 * time.Millisecond)
	if got := tt.Image().RGBAAt(10, 80); got != (color.RGBA{0, 0, 255, 255}) {
		t.Fatalf("new frame = %v", got)
	}
	if tt.rt.viewTransition != nil {
		t.Fatal("finished canvas retained")
	}
}

func TestViewTransitionInterruptionAndHidden(t *testing.T) {
	value, next := RGB(255, 0, 0), RGB(0, 0, 255)
	requested := false
	tt := coreNewTester(func(c *context) {
		if requested {
			requested = false
			makeContext(c).StartViewTransition(500*time.Millisecond, Linear, func() { value = next })
		}
		coreBox(c).Fill().Background(value)
	}, 20, 20)
	requested = true
	tt.Frame()
	tt.Advance(250 * time.Millisecond)
	before := tt.Image().RGBAAt(5, 5)
	next, requested = RGB(0, 255, 0), true
	tt.Frame()
	if got := tt.Image().RGBAAt(5, 5); got != before {
		t.Fatalf("interrupted fade jumped: %v -> %v", before, got)
	}
	tt.Advance(550 * time.Millisecond)
	if got := tt.Image().RGBAAt(5, 5); got != (color.RGBA{0, 255, 0, 255}) {
		t.Fatalf("final frame = %v", got)
	}
	tt.h.hidden = true
	next, requested = RGB(0, 0, 0), true
	tt.Frame()
	if value != next || tt.rt.viewTransition != nil {
		t.Fatal("hidden window must apply immediately")
	}
}
