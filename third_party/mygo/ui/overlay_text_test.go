package ui

import "testing"

func TestOverlayResolvedTextDuringBuild(t *testing.T) {
	want := RGB(30, 70, 120)
	var font Font
	var color Color
	coreNewTester(func(c *context) {
		c.root.Font("monospace").FontSize(23).Bold().TextColor(want)
		coreOverlay(c, func() {
			coreColumn(c).Children(func() {
				font, color = coreBox(c).ResolvedText()
			})
		})
	}, 200, 100)
	if font.Family != "monospace" || font.Size != 23 || font.Weight != 700 || color != want {
		t.Fatalf("overlay build font/color: %+v %+v", font, color)
	}
}

func TestOverlayThemedTextDuringBuild(t *testing.T) {
	window, outer, inner := LightTheme(), DarkTheme(), LightTheme()
	window.Font, window.FontSize, window.Text = "sans-serif", 14, RGB(40, 50, 60)
	outer.Font, outer.FontSize, outer.Text = "monospace", 21, RGB(60, 100, 160)
	inner.Font, inner.FontSize, inner.Text = "serif", 29, RGB(120, 180, 80)
	explicitColor := RGB(210, 80, 110)
	type sample struct {
		name    string
		node    *node
		font    Font
		color   Color
		theme   *Theme
		painted *Theme
	}
	var samples []*sample
	coreNewTester(func(c *context) {
		samples = nil
		c.SetTheme(window)
		capture := func(name string, want *Theme, explicit bool) {
			e := coreBox(c).Absolute().Left(float32(len(samples)) * 32).Top(5)
			if explicit {
				e.Font("system-ui").FontSize(17).TextColor(explicitColor)
			}
			font, color := e.ResolvedText()
			s := &sample{name: name, node: e, font: font, color: color, theme: want}
			e.Size(font.Size, font.Size).Draw(func(p *Painter, _ Rect) { s.painted = p.Theme() })
			samples = append(samples, s)
		}
		coreThemed(c, outer).Children(func() {
			coreOverlay(c, func() {
				capture("outer overlay", outer, false)
				capture("explicit style", outer, true)
				coreThemed(c, inner).Children(func() {
					capture("inner theme", inner, false)
					coreOverlay(c, func() { capture("inner overlay", inner, false) })
				})
				capture("outer restored", outer, false)
				coreThemed(c, nil).Children(func() { capture("nil theme", outer, false) })
			})
		})
		coreOverlay(c, func() { capture("window restored", window, false) })
	}, 300, 100)
	for _, s := range samples {
		family, size, color := s.theme.Font, s.theme.FontSize, s.theme.Text
		if s.name == "explicit style" {
			family, size, color = "system-ui", 17, explicitColor
		}
		if s.font.Family != family || s.font.Size != size || s.color != color {
			t.Errorf("%s build font/color: %+v %+v; want %q %v %+v", s.name, s.font, s.color, family, size, color)
		}
		if font, color := s.node.ResolvedText(); font != s.font || color != s.color {
			t.Errorf("%s text style changed after building: %+v %+v", s.name, font, color)
		}
		if s.painted != s.theme {
			t.Errorf("%s painted with theme %p, want %p", s.name, s.painted, s.theme)
		}
	}
}
