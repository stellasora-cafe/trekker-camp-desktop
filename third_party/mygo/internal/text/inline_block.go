package text

import (
	"encoding/json"
	"math"
	"slices"
)

// InlineBox describes an atomic inline container in the original rune stream.
// Descendant boxes follow their ancestors; ranges may nest but never cross.
type InlineBox struct {
	Start, End                                      int
	Spans                                           []byte // encoded spans; JSON retains their arbitrary binary bytes
	Style                                           Style
	Align                                           Align
	NoWrap, Anywhere, BreakAll, KeepSpaces, PreWrap bool
	Insets                                          [4]float32
}

func EncodeInlineBoxes(boxes []InlineBox) string {
	if len(boxes) == 0 {
		return ""
	}
	b, _ := json.Marshal(boxes)
	return string(b)
}

func decodeInlineBoxes(encoded string) []InlineBox {
	var boxes []InlineBox
	_ = json.Unmarshal([]byte(encoded), &boxes)
	return boxes
}

// InlineBoxLayout locates a container and its own paragraph in its parent's
// coordinates. Its text remains the original text, with rune offsets relative
// to Start, including nested containers, objects and line breaks.
type InlineBoxLayout struct {
	Start, End         int
	Rect               Rect
	ContentX, ContentY float32
	Layout             *Layout
}

func (s *System) inlineBlockLayout(p Params) *cached {
	runes := []rune(p.Text)
	spans := DecodeSpans(p.Spans)
	boxes := decodeInlineBoxes(p.InlineBoxes)
	var token []rune
	var offsets []int
	var styles []Span
	var laid []InlineBoxLayout
	var tokenBoxes []int
	appendText := func(from, to int) {
		for at := from; at < to; {
			sp, end := Span{}, to
			if k := spanIndex(spans, at); k < len(spans) {
				sp = spans[k]
				end = min(end, sp.End)
				start := 0
				if k > 0 {
					start = spans[k-1].End
				}
				if at > start {
					sp.Leading = 0
				}
				if end < sp.End {
					sp.Trailing = 0
				}
			}
			for k := at; k < end; k++ {
				token = append(token, runes[k])
				offsets = append(offsets, k)
			}
			sp.End = len(token)
			styles = append(styles, sp)
			at = end
		}
	}
	pos := 0
	for i := 0; i < len(boxes); {
		box := boxes[i]
		if box.Start < pos || box.Start < 0 || box.End > len(runes) || box.End <= box.Start {
			i++
			continue
		}
		appendText(pos, box.Start)
		j := i + 1
		var nested []InlineBox
		for j < len(boxes) && boxes[j].Start < box.End {
			child := boxes[j]
			child.Start -= box.Start
			child.End -= box.Start
			nested = append(nested, child)
			j++
		}
		childSpans := spansIn(spans, box.Start, box.End)
		if box.Spans != nil {
			childSpans = DecodeSpans(string(box.Spans))
		}
		child := Params{Text: string(runes[box.Start:box.End]), Style: box.Style, Spans: EncodeSpans(childSpans),
			NoWrap: box.NoWrap, BreakAll: p.BreakAll || box.BreakAll, KeepSpaces: box.KeepSpaces, PreWrap: box.PreWrap, InlineBoxes: EncodeInlineBoxes(nested), NoBreakWords: !box.Anywhere, RTL: p.RTL, Align: box.Align}
		wide := s.layout(child)
		width := wide.layout.Width
		in := box.Insets
		available := max(p.Width-in[1]-in[3], 1)
		if p.Width > 0 && !box.NoWrap && width > available {
			narrow := child
			narrow.Width = 1
			minWidth := s.layout(narrow).layout.Width
			width = min(width, max(minWidth, available))
		}
		child.Width = width
		inner := &s.layout(child).layout
		baseline := inner.Height
		if n := len(inner.Lines); n > 0 {
			baseline = inner.Lines[n-1].Baseline
		}
		w, h := width+in[1]+in[3], inner.Height+in[0]+in[2]
		tokenBoxes = append(tokenBoxes, len(token))
		token = append(token, '\ufffc')
		offsets = append(offsets, box.Start)
		placeholder := Span{End: len(token), ObjectWidth: max(w, .001), ObjectHeight: baseline + in[0], ObjectDescent: max(h-baseline-in[0], 0)}
		if k := spanIndex(spans, box.Start); k < len(spans) {
			placeholder.NoBreak = spans[k].NoBreak
			if len(childSpans) > 0 {
				placeholder.Leading = spans[k].Leading - childSpans[0].Leading
			}
			for j := k; j < len(spans) && (j == k || spans[j-1].End < box.End); j++ {
				placeholder.RubyAscent = max(placeholder.RubyAscent, spans[j].RubyAscent)
				placeholder.RubyScale = max(placeholder.RubyScale, spans[j].RubyScale)
				if spans[j].End >= box.End && len(childSpans) > 0 {
					placeholder.Trailing = spans[j].Trailing - childSpans[len(childSpans)-1].Trailing
				}
			}
		}
		styles = append(styles, placeholder)
		laid = append(laid, InlineBoxLayout{Start: box.Start, End: box.End, Rect: Rect{W: w, H: h}, ContentX: in[3], ContentY: in[0], Layout: inner})
		pos = box.End
		i = j
	}
	appendText(pos, len(runes))
	offsets = append(offsets, len(runes))
	outer := p
	outer.Text = string(token)
	outer.Spans = EncodeSpans(styles)
	outer.InlineBoxes = ""
	c := s.layout(outer)
	l := &c.layout
	l.Params, l.Runes, l.Boxes = p, runes, laid
	placed := make([]bool, len(laid))
	for li := range l.Lines {
		line := &l.Lines[li]
		line.Start, line.End = offsets[line.Start], offsets[line.End]
		for gi := range line.Glyphs {
			g := &line.Glyphs[gi]
			at := g.Cluster
			g.Cluster, g.Runes = offsets[at], offsets[min(at+g.Runes, len(offsets)-1)]-offsets[at]
			if k, ok := slices.BinarySearch(tokenBoxes, at); ok {
				placed[k] = true
				b := &l.Boxes[k]
				b.Rect.X, b.Rect.Y = g.X, g.Y-(b.Layout.Lines[len(b.Layout.Lines)-1].Baseline+b.ContentY)
				b.ContentX += b.Rect.X
				b.ContentY += b.Rect.Y
				g.Box = true
			}
		}
	}
	visible := l.Boxes[:0]
	for i, b := range l.Boxes {
		if placed[i] {
			visible = append(visible, b)
		}
	}
	l.Boxes = visible
	return c
}

// PaintLayout exposes the physical text lines inside inline containers. All
// glyph clusters still address the parent's original rune stream.
func (l *Layout) PaintLayout() *Layout {
	if len(l.Boxes) == 0 {
		return l
	}
	if l.paintLayout != nil {
		return l.paintLayout
	}
	view := *l
	view.Boxes = nil
	view.Lines = nil
	view.paintLayout = nil
	for _, line := range l.Lines {
		var glyphs []Glyph
		flush := func() {
			if len(glyphs) == 0 {
				return
			}
			part := line
			part.Glyphs = glyphs
			part.carets = nil
			part.stops = nil
			part.upstream = nil
			part.Start, part.End = math.MaxInt, 0
			x0, x1 := float32(math.MaxFloat32), float32(-math.MaxFloat32)
			for _, g := range glyphs {
				part.Start = min(part.Start, g.Cluster)
				part.End = max(part.End, g.Cluster+g.Runes)
				x0 = min(x0, g.X)
				x1 = max(x1, g.X+g.Advance)
			}
			part.X, part.Width = x0, x1-x0
			view.Lines = append(view.Lines, part)
			glyphs = nil
		}
		for _, g := range line.Glyphs {
			if !g.Box {
				glyphs = append(glyphs, g)
				continue
			}
			flush()
			box := l.boxStarting(g.Cluster)
			if box == nil {
				continue
			}
			for _, inner := range box.Layout.PaintLayout().Lines {
				part := inner
				part.Glyphs = slices.Clone(inner.Glyphs)
				part.carets = nil
				part.stops = nil
				part.upstream = nil
				part.X += box.ContentX
				part.Y += box.ContentY
				part.Baseline += box.ContentY
				part.Start += box.Start
				part.End += box.Start
				for j := range part.Glyphs {
					part.Glyphs[j].X += box.ContentX
					part.Glyphs[j].Y += box.ContentY
					part.Glyphs[j].Cluster += box.Start
				}
				view.Lines = append(view.Lines, part)
			}
		}
		flush()
		if len(line.Glyphs) == 0 {
			view.Lines = append(view.Lines, line)
		}
	}
	// Inline boxes are isolated bidi runs. Their outer glyphs are in visual
	// order, while caret navigation and selection retain logical text order.
	slices.SortStableFunc(view.Lines, func(a, b Line) int { return a.Start - b.Start })
	l.paintLayout = &view
	return &view
}

func (l *Layout) boxStarting(start int) *InlineBoxLayout {
	for i := range l.Boxes {
		if l.Boxes[i].Start == start {
			return &l.Boxes[i]
		}
	}
	return nil
}

// InlineBoxBounds returns the outer rectangle of the exact container range.
func (l *Layout) InlineBoxBounds(start, end int) (Rect, bool) {
	for i := range l.Boxes {
		b := &l.Boxes[i]
		if b.Start == start && b.End == end {
			return b.Rect, true
		}
		if start >= b.Start && end <= b.End {
			if r, ok := b.Layout.InlineBoxBounds(start-b.Start, end-b.Start); ok {
				r.X += b.ContentX
				r.Y += b.ContentY
				return r, true
			}
		}
	}
	return Rect{}, false
}

// Inline boxes hit-test their own lines. Outside them, choose the closest
// physical line rectangle, accounting for siblings on the same outer line.
func (l *Layout) inlineBoxPositionAt(x, y float32) CaretPosition {
	for _, box := range l.Boxes {
		r := box.Rect
		if x >= r.X && x <= r.X+r.W && y >= r.Y && y <= r.Y+r.H {
			p := box.Layout.PositionAt(x-box.ContentX, y-box.ContentY)
			p.Index += box.Start
			return p
		}
	}
	view := l.PaintLayout()
	best, distance := 0, float32(math.MaxFloat32)
	for i, line := range view.Lines {
		dx := max(line.X-x, x-line.X-line.Width, 0)
		dy := max(line.Y-y, y-line.Y-line.Height, 0)
		d := dx*dx + dy*dy
		if d < distance {
			best, distance = i, d
		}
	}
	if len(view.Lines) == 0 {
		return CaretPosition{}
	}
	lineView := *view
	lineView.Lines = view.Lines[best : best+1]
	return lineView.PositionAt(x, y)
}
