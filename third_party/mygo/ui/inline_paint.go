package ui

import (
	"math"
	"unicode/utf8"

	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/internal/text"
)

// Inline paint scopes use the paragraph's shaped glyphs. Only their painting
// is split: the layout, selection, hit boxes and accessible text stay shared.
func (e *node) inlinePaintScope() bool {
	return e.inlineBlock || e.ruby != nil || e.textGradientBoundary() || e.paintFn != nil || e.paintAfterFn != nil || e.blur > 0 || e.effect != nil || e.clipping != nil || e.flags&(flagClip|flagInvisible|flagDisabled) != 0 || e.fade() < 1
}

func (e *node) hasInlinePaintScopes() bool {
	for child := e.first; child != nil; child = child.next {
		if !child.hidden && (child.inlinePaintScope() || child.hasInlinePaintScopes()) {
			return true
		}
	}
	return false
}

type inlinePaintRange struct {
	layout text.Layout
	lines  []text.Line
	glyphs [][]text.Glyph
}

type inlineParagraphPaint struct {
	paragraph *node
	style     textStyle
	spans     spanPaint
	x, y      float32
	next      int
}

// paintInlineParagraph is selected only when an inline node has a paint
// scope. Span opacity stays available to layout/inspection; these paint spans
// omit group opacity, which the scope's layer applies exactly once.
func (p *Painter) paintInlineParagraph(e *node, ts textStyle) {
	sc := e.st.spanCache()
	sc.paintInline = appendInlinePaintSpans(sc.paintInline[:0], e, Span{}, true)
	sc.paintStyles = sc.paintStyles[:0]
	offset := 0
	for _, span := range sc.paintInline {
		offset += utf8.RuneCountInString(span.Text)
		sc.paintStyles = append(sc.paintStyles, spanStyle(span, offset))
	}
	paint := inlineParagraphPaint{paragraph: e, style: ts, spans: spanPaint{spans: sc.paintInline, styles: sc.paintStyles, rawColorOnly: true}, x: e.x + e.contentX(), y: e.y + e.contentY()}
	p.paintInlineContents(&paint, e, 0, len(e.tl.Runes))
}

func appendInlinePaintSpans(out []Span, e *node, inherited Span, root bool) []Span {
	base := inherited
	if !root {
		own, _ := e.inlineSpanStyle(false)
		base = over(own, inherited)
	}
	own := e.text
	if e.paragraph {
		own = e.ownText
	}
	if e.ownSpans == nil {
		if own != "" || !root && e.first == nil {
			out = append(out, over(Span{Text: own}, base))
		}
	} else {
		for _, s := range e.ownSpans {
			s.gradientColor = s.Color.A > 0
			out = append(out, over(s, base))
		}
	}
	for child := e.first; child != nil; child = child.next {
		if !child.hidden {
			out = appendInlinePaintSpans(out, child, base, false)
		}
	}
	return out
}

func (p *Painter) paintInlineContents(paint *inlineParagraphPaint, e *node, start, end int) {
	cursor := start
	var visit func(*node, int)
	visit = func(parent *node, offset int) {
		for child := parent.first; child != nil; child = child.next {
			a, b := offset+child.runes[0], offset+child.runes[1]
			if child.hidden || a == b {
				continue
			}
			if child.inlinePaintScope() {
				p.paintInlineRange(paint, cursor, a)
				p.paintInlineScopeNode(paint, child, a, b)
				cursor = b
			} else {
				visit(child, a)
			}
		}
	}
	visit(e, start)
	p.paintInlineRange(paint, cursor, end)
}

// A range is a view of the original glyph positions, not a second shaping.
// Keep the scratch lines/glyphs per paragraph so animation frames reuse them.
func (p *Painter) paintInlineRange(paint *inlineParagraphPaint, start, end int) {
	if start >= end {
		return
	}
	e := paint.paragraph
	sc := e.st.spanCache()
	if paint.next == len(sc.paintRanges) {
		sc.paintRanges = append(sc.paintRanges, &inlinePaintRange{})
	}
	r := sc.paintRanges[paint.next]
	paint.next++
	sourceLayout := e.tl.PaintLayout()
	r.layout = *sourceLayout
	r.lines = r.lines[:0]
	for i, source := range sourceLayout.Lines {
		for len(r.glyphs) <= i {
			r.glyphs = append(r.glyphs, nil)
		}
		glyphs := r.glyphs[i][:0]
		x0, x1 := float32(math.MaxFloat32), float32(-math.MaxFloat32)
		for _, g := range source.Glyphs {
			if g.Cluster >= start && g.Cluster < end {
				glyphs = append(glyphs, g)
				x0 = min(x0, g.X)
				x1 = max(x1, g.X+g.Advance)
			}
		}
		r.glyphs[i] = glyphs
		if len(glyphs) == 0 {
			continue
		}
		line := source
		line.Glyphs = glyphs
		line.Start = max(source.Start, start)
		line.End = min(source.End, end)
		line.X = x0
		line.Width = x1 - x0
		r.lines = append(r.lines, line)
	}
	r.layout.Lines = r.lines
	l, ts, sp := &r.layout, paint.style, &paint.spans
	p.textBackgrounds(l, paint.x, paint.y, ts, sp)
	if ed := e.st.editor; ed != nil && e.flags&flagSelectable != 0 && p.rt.textSelectionVisible(e.st) {
		for _, selected := range ed.selectedRanges() {
			a, b := max(selected.Start, start), min(selected.End, end)
			if a < b {
				for _, box := range e.tl.Selection(a, b) {
					p.Fill(Rect{paint.x + box.X, paint.y + box.Y, box.W, box.H}, ts.selectionColor(p.Theme()), 0)
				}
			}
		}
	}
	p.textForeground(l, paint.x, paint.y, ts.color, ts, sp)
	if e.objectFn != nil {
		paintObjects(p, l, paint.x, paint.y, e.objectFn)
	}
}

func inlineRelative(paragraph, e *node) (scene.Affine, bool) {
	inv, ok := paragraph.world.Inverse()
	if !ok {
		return scene.Affine{}, false
	}
	return inv.Mul(e.world), true
}

func (p *Painter) paintInlineScopeNode(paint *inlineParagraphPaint, e *node, start, end int) {
	if e.flags&flagInvisible != 0 || e.fade() <= .001 {
		return
	}
	relative, ok := inlineRelative(paint.paragraph, e)
	if !ok {
		return
	}
	base, savedClip, savedOpacity := p.transform, p.clip, p.opacity
	savedGradient := p.textGradient
	if e.textGradientBoundary() {
		p.textGradient = e.resolvedTextGradient()
	}
	defer func() { p.transform, p.clip, p.opacity, p.textGradient = base, savedClip, savedOpacity, savedGradient }()
	if e.flags&flagDisabled != 0 && !e.undimmed && !e.theme().DisableAutoDimming {
		p.opacity *= .5
	}
	if e.clipping != nil {
		p.transform = base.Mul(relative)
		p.clipSelf(e)
		p.transform = base
		defer func() { p.popClip() }()
	}
	if e.effect != nil || e.blur > 0 || e.fade() < 1 {
		if p.paintInlineLayer(paint, e, start, end, relative) {
			return
		}
	}
	p.opacity *= e.fade()
	p.paintInlineBody(paint, e, start, end, relative)
}

func (p *Painter) paintInlineBody(paint *inlineParagraphPaint, e *node, start, end int, relative scene.Affine) {
	base, savedClip := p.transform, p.clip
	box := Rect{e.x, e.y, e.w, e.h}
	if e.inlineBlock {
		p.transform = base.Mul(relative)
		for _, sh := range e.shadows {
			if !sh.inset {
				p.shadow(box, e.radius, sh)
			}
		}
		p.background(e, box, true)
		for _, sh := range e.shadows {
			if sh.inset {
				p.insetShadow(box, e.radius, e.border, sh)
			}
		}
		p.transform = base
	}
	if e.paintFn != nil {
		p.transform = base.Mul(relative)
		savedInline, savedGradient := p.inline, p.textGradient
		p.inline, p.textGradient = nil, nil
		p.draw(e, box, false)
		p.inline, p.textGradient = savedInline, savedGradient
		p.transform = base
	}
	if e.flags&flagClip != 0 {
		p.transform = base.Mul(relative)
		r, rad := e.clipRect()
		p.pushClip(r, rad)
		p.transform = base
	}
	p.paintInlineContents(paint, e, start, end)
	if e.ruby != nil {
		p.transform = base.Mul(relative)
		savedInline := p.inline
		p.inline = nil
		e.paintRuby(p)
		p.inline = savedInline
		p.transform = base
	}
	if e.flags&flagClip != 0 {
		p.popClip()
		p.clip = savedClip
	}
	if e.paintAfterFn != nil {
		p.transform = base.Mul(relative)
		savedInline, savedGradient := p.inline, p.textGradient
		p.inline, p.textGradient = nil, nil
		p.draw(e, box, true)
		p.inline, p.textGradient = savedInline, savedGradient
		p.transform = base
	}
}

func (p *Painter) paintInlineLayer(paint *inlineParagraphPaint, e *node, start, end int, relative scene.Affine) bool {
	inv, ok := relative.Inverse()
	if !ok {
		return true
	}
	box := Rect{e.x, e.y, e.w, e.h}
	if box.W <= 0 || box.H <= 0 {
		return true
	}
	content := box
	// Descendants may move beyond the inline box. Allocate for that ink before
	// drawing it, including nested blur/shadows, in this scope's own space.
	var ink func(*node)
	ink = func(n *node) {
		if n.hidden || n.flags&flagInvisible != 0 {
			return
		}
		m := n.shadowMargin() + 3*n.blur
		if fx := n.effect; fx != nil {
			margin := fx.EffectMargin()
			m = max(m, margin.X, margin.Y)
		}
		r := Rect{n.x - m, n.y - m, n.w + 2*m, n.h + 2*m}
		if childRelative, valid := inlineRelative(paint.paragraph, n); valid {
			content = inkUnion(content, transformRect(inv.Mul(childRelative), r))
		}
		for child := n.first; child != nil; child = child.next {
			ink(child)
		}
	}
	ink(e)
	content = p.inkArea(e, content)
	margin := Point{3 * e.blur, 3 * e.blur}
	if e.effect != nil {
		margin = e.effect.EffectMargin()
	}
	from := Rect{content.X - margin.X, content.Y - margin.Y, content.W + 2*margin.X, content.H + 2*margin.Y}
	var cv *Canvas
	var x0, y0 float32
	var w, h int
	base := p.transform
	mark := paint.next
	for again := false; ; again = true {
		cv, x0, y0, w, h = p.layer(e, from, p.scale)
		if cv == nil {
			return false
		}
		paint.next = mark
		cv.drawPart(p, p.scale, w, h, func(cp *Painter) {
			cp.grayscale = true
			cp.inline = paint.paragraph
			cp.textGradient = p.textGradient
			cp.transform = scene.Translation(-x0/p.scale, -y0/p.scale).Mul(inv)
			cp.paintInlineBody(paint, e, start, end, relative)
		})
		actual := p.keepInk(e, cv, x0, y0, p.scale)
		if again || e.effect != nil {
			break
		}
		content = inkUnion(content, actual)
		more := Rect{content.X - margin.X, content.Y - margin.Y, content.W + 2*margin.X, content.H + 2*margin.Y}
		if layerHolds(x0, y0, w, h, p.scale, more) {
			break
		}
		from = more
	}
	p.transform = base.Mul(relative)
	if e.effect != nil {
		opacity := p.opacity
		p.opacity *= e.fade()
		savedInline, savedGradient := p.inline, p.textGradient
		p.inline, p.textGradient = nil, nil
		e.effect.PaintEffect(p, box, cv.region(w, h), Point{box.X*p.scale - x0, box.Y*p.scale - y0})
		p.inline, p.textGradient = savedInline, savedGradient
		p.opacity = opacity
	} else {
		p.layerOp(cv, x0, y0, w, h, p.opacity*e.fade(), e.blur*p.scale, 1, false)
	}
	p.transform = base
	return true
}
