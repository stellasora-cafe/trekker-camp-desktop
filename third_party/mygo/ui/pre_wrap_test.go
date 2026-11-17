package ui

import "testing"

func TestInlineBlockInheritsPreWrap(t *testing.T) {
	var para *node
	coreNewTester(func(c *context) {
		para = coreRichText(c).PreWrap().Width(80).FontSize(20).Children(func() {
			coreRichText(c).InlineBlock().Children(func() { coreText(c, "A   \nB") })
		})
	}, 200, 160)
	box := para.tl.Boxes[0]
	if !box.Layout.Params.PreWrap {
		t.Fatal("pre-wrap not inherited")
	}
	if len(box.Layout.Selection(1, 4)) != 1 || box.Layout.Selection(1, 4)[0].W <= 0 {
		t.Fatal("hanging spaces lost inside inline block")
	}
	if len(box.Layout.Lines) != 2 {
		t.Fatal("preserved newline lost")
	}
}

func TestContextPixelScale(t *testing.T) {
	var observed float32
	zoom := float32(1)
	tt := coreNewTester(func(c *context) { c.SetZoom(zoom); observed = c.PixelScale(); coreText(c, "scale") }, 200, 120)
	for _, scale := range []float32{1, 1.25, 1.5, 2, 1} {
		tt.SetScale(scale)
		if observed != scale {
			t.Fatalf("scale=%v, observed=%v", scale, observed)
		}
	}
	zoom = 1.25
	tt.SetScale(1.5)
	if observed != 1.875 {
		t.Fatalf("display DPI * zoom = %v, want 1.875", observed)
	}

}

func TestInlineBlockWhitespaceOverride(t *testing.T) {
	var paragraph *node
	coreNewTester(func(c *context) {
		paragraph = coreRichText(c).PreserveSpaces().Children(func() {
			coreRichText(c).PreWrap().Children(func() {
				coreText(c, "A   ").InlineBlock()
				coreText(c, "B   ").InlineBlock().PreserveSpaces()
			})
		})
	}, 240, 120)
	first, second := paragraph.tl.Boxes[0].Layout, paragraph.tl.Boxes[1].Layout
	if !first.Params.PreWrap || first.Params.KeepSpaces {
		t.Fatal("nearest pre-wrap override lost")
	}
	if second.Params.PreWrap || !second.Params.KeepSpaces {
		t.Fatal("nearest break-spaces override lost")
	}
}
