package ui

import "github.com/egoist/mygo/internal/scene"

// TextGradient fills the element's glyphs and its descendants' glyphs with a
// linear gradient across the element's layout box. It replaces the glyph fill
// color, leaving backgrounds, inline objects, shadows, strokes, decorations
// and Draw callbacks in their own colors. Descendants inherit the gradient;
// calling TextGradient on one gives its glyphs a gradient of their own.
func (e *node) TextGradient(g LinearGradient) *node {
	e.textGradient = &g
	return e
}

type textGradientPaint struct {
	gradient LinearGradient
	box      Rect
}

func (e *node) resolvedTextGradient() *textGradientPaint {
	for n := e; n != nil; n = n.parent {
		if n.textGradient != nil {
			return &textGradientPaint{gradient: *n.textGradient, box: Rect{n.x, n.y, n.w, n.h}}
		}
		if n.ts.set&setColor != 0 && n.ts.color.A > 0 {
			return nil
		}
	}
	return nil
}

func (e *node) textGradientBoundary() bool {
	if e.textGradient != nil {
		return true
	}
	if e.ts.set&setColor == 0 || e.ts.color.A == 0 {
		return false
	}
	return e.parent != nil && e.parent.resolvedTextGradient() != nil
}

func (sp *spanPaint) overridesTextGradient(i int) bool {
	if sp == nil || i < 0 {
		return false
	}
	if sp.rawColorOnly {
		return sp.spans[i].gradientColor
	}
	return sp.spans[i].Color.A > 0
}

func (p *Painter) inlineGlyphs(start, end int32, gradient bool) {
	if end <= start {
		return
	}
	if gradient {
		p.gradientGlyphs(start, end)
	} else {
		p.add(scene.Op{Kind: scene.OpGlyphs, Start: start, End: end})
	}
}

func (p *Painter) gradientGlyphs(start, end int32) {
	if end <= start {
		return
	}
	g := p.textGradient
	op := scene.Op{Kind: scene.OpGlyphs, Start: start, End: end, Rect: p.snap(g.box), Opacity: p.opacity}
	p.gradient(&op, g.gradient)
	op.Wide = p.wide(g.gradient.From, g.gradient.To, Color{})
	p.add(op)
}
