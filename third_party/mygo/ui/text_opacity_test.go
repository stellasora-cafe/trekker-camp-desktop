package ui

import "testing"

func TestTranslucentTextUsesGrayscale(t *testing.T) {
	for _, mode := range []string{"root", "inline", "span"} {
		t.Run(mode, func(t *testing.T) {
			img := inlineScopeImage(func(c *context) {
				e := coreRichText(c).FontSize(30).TextColor(RGB(0, 0, 0))
				switch mode {
				case "root":
					e.Opacity(.5).Children(func() { coreText(c, "MMMM inline text") })
				case "inline":
					e.Children(func() { coreText(c, "MMMM inline text").Opacity(.5) })
				case "span":
					e.Children(func() { coreRichText(c, Span{Text: "MMMM inline text", Color: RGBA(0, 0, 0, .5)}) })
				}
			})
			ink := 0
			for y := 0; y < img.Bounds().Dy(); y++ {
				for x := 0; x < img.Bounds().Dx(); x++ {
					c := img.RGBAAt(x, y)
					if c.R < 250 || c.G < 250 || c.B < 250 {
						ink++
						if c.R != c.G || c.G != c.B {
							t.Fatalf("translucent %s has LCD fringe at %d,%d: %v", mode, x, y, c)
						}
					}
				}
			}
			if ink == 0 {
				t.Fatal("text not painted")
			}
		})
	}
}
