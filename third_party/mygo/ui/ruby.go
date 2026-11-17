package ui

import (
	"math"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/internal/text"
)

// Ruby annotates this inline element with half-size text above its base. The
// base stays the original rich subtree, retaining links, selection and styles.
// Both runs use space-around alignment and move between lines together.
func (e *node) Ruby(annotation string) *node {
	e.ruby = &rubyAnnotation{text: annotation}
	e.noWrap = true
	return e
}

type rubyAnnotation struct {
	text                                   string
	layout                                 *text.Layout
	offset, width, inset, overhang, anchor float32
	color                                  Color
	style                                  textStyle
}

func rubyIdeograph(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

func rubyGroups(runes []rune) ([]int, int) {
	groups := make([]int, len(runes))
	n := 0
	for i, r := range runes {
		if i > 0 && !unicode.Is(unicode.Mn, r) && !unicode.Is(unicode.Me, r) && (rubyIdeograph(r) || rubyIdeograph(runes[i-1]) || unicode.IsSpace(r) || unicode.IsSpace(runes[i-1])) {
			n++
		}
		groups[i] = n
	}
	if len(runes) == 0 {
		return groups, 0
	}
	return groups, n + 1
}

func rubyExpand(spans []Span, extra float32) ([]Span, float32) {
	if extra <= 0 {
		return spans, 0
	}
	var body strings.Builder
	for _, sp := range spans {
		body.WriteString(sp.Text)
	}
	groups, count := rubyGroups([]rune(body.String()))
	if count == 0 {
		return spans, 0
	}
	unit := extra / float32(count)
	var out []Span
	position := 0
	for _, sp := range spans {
		runes := []rune(sp.Text)
		for start := 0; start < len(runes); {
			end := start + 1
			for end < len(runes) && groups[position+end] == groups[position+start] {
				end++
			}
			part := sp
			part.Text = string(runes[start:end])
			if start > 0 {
				part.leading = 0
			}
			if end < len(runes) {
				part.trailing = 0
			}
			if position+start == 0 || groups[position+start-1] != groups[position+start] {
				part.leading += unit / 2
			}
			if position+end == len(groups) || groups[position+end-1] != groups[position+end] {
				part.trailing += unit / 2
			}
			out = append(out, part)
			start = end
		}
		position += len(runes)
	}
	return out, unit / 2
}

func (e *node) layoutRuby(spans []Span) []Span {
	if e.ruby == nil || e.ruby.text == "" || e.text == "" || len(spans) == 0 {
		return spans
	}
	r := e.ruby
	ts := e.resolvedText()
	style := text.Style{Family: ts.family, Size: ts.size, Weight: ts.weight, Italic: ts.italic, LineHeight: ts.lineHeight, LetterSpacing: ts.spacing, Features: ts.features, Lang: ts.lang, TabSize: ts.tabSize}
	if ts.fixedLine && ts.lineHeight > 0 {
		style.LineHeight = ts.lineHeight / style.FontSize()
	}
	base := text.Shared().Layout(text.Params{Text: e.text, Style: style, Spans: encodeSpans(spans), InlineBoxes: e.inlineBoxSpecs(), NoWrap: true, KeepSpaces: true})
	annotationStyle := style
	annotationStyle.Size *= .5
	annotationStyle.LineHeight = 0
	annotation := text.Shared().Layout(text.Params{Text: r.text, Style: annotationStyle, NoWrap: true, KeepSpaces: true})
	r.width = max(base.Width, annotation.Width)
	original := slices.Clone(spans)
	spans, r.inset = rubyExpand(spans, r.width-base.Width)
	e.expandNestedRuby(original, spans)
	// Overhang is limited to half an annotation em and half the adjoining
	// text run; an edge or an inline object provides no overhang space.
	start, end := e.rubyNeighbors(style)
	if len(spans) == 1 && spans[0].ObjectWidth > 0 && spans[0].ObjectWidth <= .001 {
		start, end = 0, 0
	}
	overhang := min(r.inset, annotationStyle.FontSize()/2)
	r.overhang = min(overhang, start/2)
	spans[0].leading -= r.overhang
	spans[len(spans)-1].trailing -= min(overhang, end/2)
	r.anchor = spans[0].leading + r.overhang
	baseAscent := float32(0)
	_, _, scale := e.c.rt.size()
	if scale <= 0 {
		scale = 1
	}
	ceil := func(v float32) float32 { return float32(math.Ceil(float64(v*scale))) / scale }
	for _, line := range base.Lines {
		for _, glyph := range line.Glyphs {
			if glyph.Box {
				for _, box := range base.Boxes {
					if box.Start == glyph.Cluster && len(box.Layout.Lines) > 0 {
						baseline := box.ContentY - box.Rect.Y + box.Layout.Lines[len(box.Layout.Lines)-1].Baseline
						baseAscent = max(baseAscent, float32(math.Round(float64(baseline*scale)))/scale)
					}
				}
				continue
			}
			if glyph.Object {
				baseAscent = max(baseAscent, line.Ascent)
				continue
			}
			if glyph.Font != nil {
				a, _ := glyph.Font.TypographicMetrics()
				baseAscent = max(baseAscent, ceil(a)+line.Baseline-glyph.Y)
			}
		}
	}
	for _, span := range spans {
		baseAscent = max(baseAscent, span.rubyAscent)
	}
	if baseAscent == 0 {
		baseAscent, _ = text.Shared().TypographicMetrics(style)
	}
	annotationAscent, annotationDescent := text.Shared().TypographicMetrics(annotationStyle)
	annotationAscent, annotationDescent = ceil(annotationAscent), ceil(annotationDescent)
	r.offset = baseAscent + annotationDescent
	for i := range spans {
		spans[i].rubyAscent = max(spans[i].rubyAscent, r.offset+annotationAscent)
		spans[i].rubyScale = scale
	}
	annotationSpans, _ := rubyExpand([]Span{{Text: r.text}}, r.width-annotation.Width)
	r.layout = text.Shared().Layout(text.Params{Text: r.text, Style: annotationStyle, Spans: encodeSpans(annotationSpans), NoWrap: true, KeepSpaces: true})
	r.color, r.style = ts.color, ts
	return spans
}

// Outer ruby justification widens the edge spaces of an inner annotation.
// The inner base keeps every outer gap, while its annotation receives the
// first and last gap, following the inline ruby formatting context.
func (e *node) expandNestedRuby(before, after []Span) {
	edges := func(spans []Span, from, to int) (leading, trailing float32) {
		at := 0
		for _, span := range spans {
			end := at + utf8.RuneCountInString(span.Text)
			if at == from {
				leading = span.leading
			}
			if end == to {
				trailing = span.trailing
			}
			at = end
		}
		return
	}
	var visit func(*node, int)
	visit = func(parent *node, offset int) {
		for child := parent.first; child != nil; child = child.next {
			if child.inlineBlock {
				continue
			}
			a, b := offset+child.runes[0], offset+child.runes[1]
			if r := child.ruby; r != nil && r.layout != nil {
				left, right := edges(after, a, b)
				oldLeft, oldRight := edges(before, a, b)
				left -= oldLeft
				right -= oldRight
				r.anchor += left
				if left+right > 0 {
					style := r.layout.Params.Style
					natural := text.Shared().Layout(text.Params{Text: r.text, Style: style, NoWrap: true, KeepSpaces: true})
					spans, _ := rubyExpand([]Span{{Text: r.text}}, r.layout.Width+left+right-natural.Width)
					r.layout = text.Shared().Layout(text.Params{Text: r.text, Style: style, Spans: encodeSpans(spans), NoWrap: true, KeepSpaces: true})
				}
			}
			visit(child, a)
		}
	}
	visit(e, 0)
}

func (e *node) rubyNeighbors(style text.Style) (before, after float32) {
	neighbor := func(backward bool) *node {
		for at := e; at.isInline(); at = at.parent {
			var candidate *node
			if backward {
				for child := at.parent.first; child != nil && child != at; child = child.next {
					if !child.hidden && child.text != "" {
						candidate = child
					}
				}
			} else {
				for child := at.next; child != nil; child = child.next {
					if !child.hidden && child.text != "" {
						candidate = child
						break
					}
				}
			}
			if candidate != nil {
				for candidate.first != nil && candidate.ruby == nil && !candidate.inlineBlock {
					var leaf *node
					for child := candidate.first; child != nil; child = child.next {
						if !child.hidden && child.text != "" {
							leaf = child
							if !backward {
								break
							}
						}
					}
					if leaf == nil {
						break
					}
					candidate = leaf
				}
				return candidate
			}
			if at.parent.inlineBlock || at.parent.ruby != nil {
				break
			}
		}
		return nil
	}
	measure := func(n *node) float32 {
		if n == nil || n.ruby != nil || n.inlineBlock || n.text == "" {
			return 0
		}
		ts := n.resolvedText()
		if ts.size > style.FontSize() {
			return 0
		}
		for _, sp := range n.spans {
			if sp.ObjectWidth > 0 {
				return 0
			}
		}
		return text.Shared().Layout(text.Params{Text: n.text, Style: text.Style{Family: ts.family, Size: ts.size, Weight: ts.weight, Italic: ts.italic}, NoWrap: true, KeepSpaces: true}).Width
	}
	return measure(neighbor(true)), measure(neighbor(false))
}

func (e *node) paintRuby(p *Painter) {
	r := e.ruby
	if r == nil || r.layout == nil || len(r.layout.Lines) == 0 {
		return
	}
	start := 0
	paragraph := e
	var ancestors []*node
	for paragraph.isInline() {
		start += paragraph.runes[0]
		paragraph = paragraph.parent
		if paragraph.inlineBlock {
			ancestors = append(ancestors, paragraph)
		}
	}
	if paragraph.tl == nil {
		return
	}
	layout := paragraph.tl
	ox, oy := paragraph.x+paragraph.contentX(), paragraph.y+paragraph.contentY()
	origin := 0
	for i := len(ancestors) - 1; i >= 0; i-- {
		n := ancestors[i]
		a := 0
		for node := n; node != paragraph; node = node.parent {
			a += node.runes[0]
		}
		for _, box := range layout.Boxes {
			if box.Start == a-origin && box.End-box.Start == len([]rune(n.text)) {
				origin += box.Start
				ox += box.ContentX
				oy += box.ContentY
				layout = box.Layout
				break
			}
		}
	}
	start -= origin
	for _, line := range layout.Lines {
		for _, glyph := range line.Glyphs {
			if glyph.Cluster == start {
				x := ox + glyph.X - r.anchor
				y := oy + line.Baseline - r.offset - r.layout.Lines[0].Baseline
				p.textForeground(r.layout, x, y, r.color, r.style, nil)
				return
			}
		}
	}
}
