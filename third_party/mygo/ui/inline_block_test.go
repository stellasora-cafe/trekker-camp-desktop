package ui

import (
	"testing"

	"github.com/egoist/mygo/internal/text"
)

func TestInlineBlockInteraction(t *testing.T) {
	var paragraph, block, link *node
	var objects []Rect
	tt := coreNewTester(func(c *context) {
		paragraph = coreRichText(c).Width(180).FontSize(20).PreserveSpaces().Selectable().Children(func() {
			coreText(c, "Before ")
			block = coreRichText(c).InlineBlock().Padding(4).Children(func() {
				link = coreLink(c, "one two three four five six seven", "https://example.com/")
				coreRichText(c, Span{Text: "\ufffc", ObjectWidth: 20, ObjectHeight: 20})
			})
			coreText(c, " after")
		}).DrawObjects(func(p *Painter, _ int, r Rect) { objects = append(objects, r); p.Fill(r, RGB(255, 0, 0), 0) })
	}, 300, 300)
	if len(block.frags) != 1 || len(link.frags) < 2 {
		t.Fatalf("box=%v link=%v", block.frags, link.frags)
	}
	if block.w > 180 || block.h <= 40 {
		t.Fatalf("not a shrink-to-fit multiline box: %v", block.frags)
	}
	if len(objects) == 0 || objects[0].Y < block.y || objects[0].Y+objects[0].H > block.y+block.h+.01 {
		t.Fatalf("object outside child layout: %v", objects)
	}
	for _, r := range link.frags {
		tt.ClickAt(center(r))
	}
	if len(tt.OpenedURLs()) != len(link.frags) {
		t.Fatal("wrapped link lost hit targets")
	}
	if paragraph.text != "Before one two three four five six seven\ufffc after" {
		t.Fatal("accessible/selected text changed")
	}
	for _, box := range paragraph.tl.Boxes {
		p := text.CaretPosition{Index: box.Start}
		for steps := 0; p.Index < box.End; steps++ {
			next := paragraph.tl.MoveCaret(p, 1)
			if next == p || next.Index < p.Index || steps > len(paragraph.text)*2 {
				t.Fatalf("caret stopped at %v => %v", p, next)
			}
			p = next
		}
	}
}

func TestInlineBlockPaintsBackgroundAndNestedObjects(t *testing.T) {
	var block Rect
	var object Rect
	img := inlineScopeImage(func(c *context) {
		coreRichText(c).Width(130).FontSize(20).Children(func() {
			coreText(c, "A ")
			coreRichText(c).InlineBlock().Padding(6).Background(RGB(10, 180, 30)).Border(2, RGB(0, 0, 255)).Children(func() {
				coreText(c, "words wrap here")
				coreRichText(c, Span{Text: "\ufffc", ObjectWidth: 20, ObjectHeight: 20}).Translate(3, 0)
			}).DrawOver(func(_ *Painter, r Rect) { block = r })
			coreText(c, " Z")
		}).DrawObjects(func(p *Painter, _ int, r Rect) { object = r; p.Fill(r, RGB(255, 0, 0), 0) })
	})
	if c := img.RGBAAt(int(block.X+3), int(block.Y+3)); c.G != 180 {
		t.Fatalf("box padding background missing: %v box=%v", c, block)
	}
	if c := img.RGBAAt(int(block.X+.5), int(block.Y+5)); c.B != 255 || c.R != 0 {
		t.Fatalf("box border missing: %v", c)
	}
	if c := img.RGBAAt(int(object.X+10+3), int(object.Y+10)); c.R != 255 || c.G != 0 {
		t.Fatalf("nested transformed object missing: %v object=%v", c, object)
	}
}

func TestInlineBlockCacheChanges(t *testing.T) {
	atomic, anywhere := false, false
	var paragraph *node
	tt := coreNewTester(func(c *context) {
		paragraph = coreRichText(c).Width(100).FontSize(20).Children(func() {
			coreText(c, "A ")
			e := coreText(c, "verylongunbrokenword")
			if atomic {
				e.InlineBlock()
			}
			if anywhere {
				e.WrapAnywhere()
			}
		})
	}, 300, 300)
	if len(paragraph.tl.Boxes) != 0 {
		t.Fatal("unexpected box")
	}
	atomic = true
	tt.Frame()
	if len(paragraph.tl.Boxes) != 1 || paragraph.tl.Boxes[0].Rect.W <= 100 {
		t.Fatal("adding inline-block reused ordinary text layout")
	}
	anywhere = true
	tt.Frame()
	if paragraph.tl.Boxes[0].Rect.W > 100 {
		t.Fatal("changing wrap policy reused min-content layout")
	}
}
