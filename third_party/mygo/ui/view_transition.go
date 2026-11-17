package ui

import (
	"slices"
	"time"

	"github.com/egoist/mygo/internal/scene"
)

// StartViewTransition paints the current view into a GPU canvas before update
// runs, then fades that canvas over the updated view. The controls keep their
// identities, focus and scroll positions. Call it while building a view, before
// changing the state the view reads. An invisible window applies update at once.
func (c *Context) StartViewTransition(duration time.Duration, ease Easing, update func()) {
	rt := c.runtime()
	if rt == nil || !rt.inFrame || duration <= 0 || rt.host.occluded() {
		if rt != nil {
			rt.viewTransition = nil
		}
		if update != nil {
			update()
		}
		return
	}
	if ease == nil {
		ease = EaseInOut
	}
	if t := rt.viewTransition; t != nil && t.pending {
		t.updates = append(t.updates, update)
		return
	}
	rt.viewTransition = &viewTransition{duration: duration, ease: ease, updates: []func(){update}, pending: true, previous: rt.viewTransition}
}

type viewTransition struct {
	canvas   *Canvas
	duration time.Duration
	ease     Easing
	start    time.Time
	updates  []func()
	pending  bool
	previous *viewTransition
	frozen   map[*scene.Image]*scene.Image
}

func (rt *engine) paintViewTransition() {
	t := rt.viewTransition
	if t == nil {
		return
	}
	s := &rt.scene
	if t.pending {
		if t.previous != nil {
			rt.viewTransition = t.previous
			rt.paintViewTransition()
			rt.viewTransition = t
		}
		// The canvas is rendered in this frame, while every glyph and nested
		// layer still holds the current view. Later frames reuse its texture.
		cv := NewCanvas(s.Width, s.Height)
		cv.img.Transient = true
		t.frozen = make(map[*scene.Image]*scene.Image)
		if t.previous != nil {
			for _, frozen := range t.previous.frozen {
				t.frozen[frozen] = frozen
			}
		}
		*cv.img.Scene = freezeTransitionScene(s, t.frozen, make(map[*scene.Atlas]*scene.Atlas))
		t.frozen[cv.img] = cv.img
		t.canvas = cv
		s.Canvases = append(s.Canvases, cv.img)
		// Showing it also makes damage tracking present the capture even when
		// the controls themselves have not changed in this frame.
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpImage, Rect: scene.Rect{W: float32(s.Width), H: float32(s.Height)},
			Src: scene.Rect{W: float32(s.Width), H: float32(s.Height)}, Image: cv.img, Opacity: 1})
		return
	}
	progress := float32(rt.c.now.Sub(t.start)) / float32(t.duration)
	if progress >= 1 || t.canvas.img.W != s.Width || t.canvas.img.H != s.Height {
		rt.viewTransition = nil
		return
	}
	if opacity := 1 - t.ease(max(progress, 0)); opacity > 0 {
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpImage, Rect: scene.Rect{W: float32(s.Width), H: float32(s.Height)},
			Src: scene.Rect{W: float32(s.Width), H: float32(s.Height)}, Image: t.canvas.img, Opacity: opacity})
	}
	rt.repainting, rt.repaintFull, rt.untracked = true, true, true
}

// Captures may later be read by a different renderer, as CapturePage draws a
// GPU window in memory. Keep its glyphs and nested canvases independent of the
// live frame, even after those layers or the transient atlas have been reused.
func freezeTransitionScene(s *scene.Scene, images map[*scene.Image]*scene.Image, atlases map[*scene.Atlas]*scene.Atlas) scene.Scene {
	frozen := *s
	frozen.Ops = slices.Clone(s.Ops)
	frozen.Glyphs = slices.Clone(s.Glyphs)
	frozen.Stops = slices.Clone(s.Stops)
	frozen.Wide = slices.Clone(s.Wide)
	frozen.Effects = slices.Clone(s.Effects)
	frozen.Words, frozen.Canvases = nil, nil
	freezeAtlas := func(a *scene.Atlas) *scene.Atlas {
		if a == nil {
			return nil
		}
		if copy := atlases[a]; copy != nil {
			return copy
		}
		copy := scene.NewAtlas(a.BPP, a.W, a.H)
		copy.Pix = slices.Clone(a.Pix)
		atlases[a] = copy
		return copy
	}
	frozen.MaskAtlas, frozen.ColorAtlas = freezeAtlas(s.MaskAtlas), freezeAtlas(s.ColorAtlas)
	freezeImage := func(img *scene.Image) *scene.Image {
		if img == nil || img.Scene == nil {
			return img
		}
		if copy := images[img]; copy != nil {
			return copy
		}
		copy := scene.NewCanvas(img.W, img.H)
		copy.Transient = true
		images[img] = copy
		*copy.Scene = freezeTransitionScene(img.Scene, images, atlases)
		return copy
	}
	for i := range frozen.Ops {
		frozen.Ops[i].Image = freezeImage(frozen.Ops[i].Image)
	}
	for i := range frozen.Effects {
		e := &frozen.Effects[i]
		e.Uniforms = slices.Clone(e.Uniforms)
		e.WideUniforms = slices.Clone(e.WideUniforms)
		for j := range e.Images {
			e.Images[j] = freezeImage(e.Images[j])
		}
	}
	return frozen
}

func (rt *engine) commitViewTransition() {
	t := rt.viewTransition
	if t == nil || !t.pending || t.canvas == nil || rt.closed || rt.closing {
		return
	}
	t.pending, t.start = false, rt.c.now
	t.previous = nil
	updates := t.updates
	t.updates = nil
	for _, update := range updates {
		if update != nil {
			update()
		}
	}
	rt.host.invalidate()
}
