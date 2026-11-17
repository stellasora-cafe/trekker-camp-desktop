package ui

import (
	"fmt"
	"testing"
)

func TestInlineNoWrap(t *testing.T) {
	for _, width := range []float32{50, 160, 300} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			var para, group, link *node
			tt := coreNewTester(func(c *context) {
				para = coreRichText(c).Width(width).FontSize(20).Selectable().Children(func() {
					coreText(c, "before ")
					group = coreRichText(c).NoWrap().Children(func() {
						coreText(c, "中文 ruby ").Bold()
						link = coreLink(c, "linked words", "https://example.com/")
						coreRichText(c, Span{Text: "\ufffc", ObjectWidth: 20, ObjectHeight: 20})
					})
					coreText(c, " after more words")
				})
			}, 400, 200)
			if len(group.frags) != 1 {
				t.Fatalf("atomic range split into %d fragments at width %v: %v", len(group.frags), width, group.frags)
			}
			if para.text != "before 中文 ruby linked words\ufffc after more words" {
				t.Fatal("layout changed accessible/selected text")
			}
			if width == 50 && group.frags[0].W <= width {
				t.Fatal("oversized range did not overflow")
			}
			tt.ClickAt(center(link.frags[0]))
			if len(tt.OpenedURLs()) != 1 {
				t.Fatal("atomic inline link lost click behavior")
			}
		})
	}
}

func TestInlineNoWrapNestedAndAdjacent(t *testing.T) {
	var first, second *node
	coreNewTester(func(c *context) {
		coreRichText(c).Width(90).FontSize(20).Children(func() {
			first = coreRichText(c).NoWrap().Children(func() {
				coreText(c, "one ")
				coreRichText(c).NoWrap().Children(func() { coreText(c, "two "); coreText(c, "three") })
			})
			second = coreRichText(c).NoWrap().Children(func() { coreText(c, "four "); coreText(c, "five") })
		})
	}, 300, 160)
	if len(first.frags) != 1 || len(second.frags) != 1 {
		t.Fatalf("ranges fragmented: %v %v", first.frags, second.frags)
	}
	if first.y == second.y {
		t.Fatal("separate atomic siblings merged")
	}
}

func TestFontXHeight(t *testing.T) {
	a := Font{Family: "system-ui", Size: 20}.Metrics()
	b := Font{Family: "system-ui", Size: 40}.Metrics()
	if a.XHeight <= 0 || a.XHeight >= a.Ascent {
		t.Fatalf("invalid x-height: %+v", a)
	}
	if abs32(b.XHeight-2*a.XHeight) > 1 {
		t.Fatalf("x-height does not scale: %+v %+v", a, b)
	}
}
