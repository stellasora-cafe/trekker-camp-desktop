package ui

import (
	"slices"

	"github.com/egoist/mygo/internal/text"
)

// InlineBlock lays out this inline subtree as a separate paragraph. The box
// occupies one position on its parent's line, aligned on its last baseline;
// its contents wrap within the available width. Padding and borders surround
// the inner paragraph. NoWrap and PreserveSpaces apply to the inner text.
func (e *node) InlineBlock() *node { e.inlineBlock = true; return e }

// WrapAnywhere allows a long word inside an inline block to wrap at grapheme
// boundaries, as CSS overflow-wrap:anywhere. It also reduces the block's
// minimum intrinsic width to a single grapheme.
func (e *node) WrapAnywhere() *node { e.inlineAnywhere = true; return e }

// WordBreakAll permits line breaks between the graphemes of words, as CSS
// word-break:break-all. NoWrap and atomic inline ranges remain unbroken.
func (e *node) WordBreakAll() *node { e.wordBreakAll = true; return e }

func (e *node) inlineBoxSpecs() string {
	var boxes []text.InlineBox
	var visit func(*node, int, bool, bool)
	visit = func(parent *node, offset int, keepSpaces, preWrap bool) {
		if parent.keepSpaces || parent.preWrap {
			keepSpaces, preWrap = parent.keepSpaces, parent.preWrap
		}
		for child := parent.first; child != nil; child = child.next {
			a, b := offset+child.runes[0], offset+child.runes[1]
			if child.hidden || a == b {
				continue
			}
			if child.inlineBlock {
				t := child.resolvedText()
				style := text.Style{Family: t.family, Size: t.size, Weight: t.weight, Italic: t.italic, LineHeight: t.lineHeight, LetterSpacing: t.spacing, Features: t.features, Lang: t.lang, TabSize: t.tabSize}
				if t.fixedLine && t.lineHeight > 0 {
					style.LineHeight = t.lineHeight / style.FontSize()
				}
				align := text.Start
				switch {
				case t.align == Center:
					align = text.Center
				case child.rtl():
					align = text.Right
					if t.align == End {
						align = text.Left
					}
				case t.align == End:
					align = text.End
				}
				encoded := child.textSpans()
				if child.first == nil {
					spans := slices.Clone(child.ownSpans)
					if spans == nil {
						spans = []Span{{Text: child.text}}
					}
					encoded = encodeSpans(child.layoutRuby(spans))
				}
				childKeep, childPre := keepSpaces, preWrap
				if child.keepSpaces || child.preWrap {
					childKeep, childPre = child.keepSpaces, child.preWrap
				}
				boxes = append(boxes, text.InlineBox{Start: a, End: b, Spans: []byte(encoded), Style: style, Align: align, NoWrap: child.noWrap, Anywhere: child.inlineAnywhere, BreakAll: child.wordBreakAll, KeepSpaces: childKeep, PreWrap: childPre,
					Insets: [4]float32{child.contentY(), child.padX() - child.contentX(), child.padY() - child.contentY(), child.contentX()}})
			}
			visit(child, a, keepSpaces, preWrap)
		}
	}
	visit(e, 0, false, false)
	return text.EncodeInlineBoxes(boxes)
}
