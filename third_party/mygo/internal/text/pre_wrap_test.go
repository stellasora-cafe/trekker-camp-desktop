package text

import (
	"runtime"
	"testing"
)

// Captured in Chromium at DPR 1.5, Segoe UI 20px, line-height 27px,
// width 50px and tab-size 2. The source preserves every rune.
func TestPreWrapChromiumGeometry(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Chromium fixture uses the Windows Segoe UI font")
	}
	s := Shared()
	s.UseTabSize()
	for _, tc := range []struct {
		text            string
		lines           int
		lastSpace, endX float32
	}{
		{"A   \nB", 2, 29.34375, 11.46875},
		{"A          B", 2, 67.69792, 11.46875},
		{"          ", 1, 54.79167, 54.79167},
		{"A \t B", 1, 27.39583, 38.86458},
	} {
		p := Params{Text: tc.text, Style: Style{Family: "Segoe UI", Size: 20, LineHeight: 1.35, TabSize: 2}, Width: 50, PreWrap: true}
		l := s.Layout(p)
		if len(l.Lines) != tc.lines {
			t.Fatalf("%q: %d lines, want %d", tc.text, len(l.Lines), tc.lines)
		}
		if abs(l.Height-float32(tc.lines)*27) > 1 {
			t.Fatalf("%q height=%v", tc.text, l.Height)
		}
		end, _, _ := l.Caret(len(l.Runes))
		if abs(end-tc.endX) > 1 {
			t.Fatalf("%q: end caret=%v, want %v", tc.text, end, tc.endX)
		}
		space := float32(0)
		for _, g := range l.Lines[0].Glyphs {
			if g.Cluster < len(l.Runes) && l.Runes[g.Cluster] == ' ' {
				space = max(space, g.X+g.Advance)
			}
		}
		if abs(space-tc.lastSpace) > 1 {
			t.Fatalf("%q: hanging space end=%v, want %v", tc.text, space, tc.lastSpace)
		}
		for i, r := range l.Runes {
			if r == ' ' {
				selection := l.Selection(i, i+1)
				if len(selection) != 1 || selection[0].W <= 0 {
					t.Fatalf("%q: space %d lost selection geometry", tc.text, i)
				}
			}
		}
	}
}

func TestPreWrapAlignmentAndIntrinsicWidth(t *testing.T) {
	s := Shared()
	p := Params{Text: "A   ", Style: Style{Size: 20}, Width: 100, PreWrap: true, Align: Center}
	l := s.Layout(p)
	plain := p
	plain.Text = "A"
	want := s.Layout(plain)
	if l.Width != want.Width || l.Lines[0].X != want.Lines[0].X {
		t.Fatal("hanging spaces changed intrinsic width or centering")
	}
	p.PreWrap = false
	p.KeepSpaces = true
	if s.Layout(p).Width <= l.Width {
		t.Fatal("pre-wrap changed existing break-spaces semantics")
	}
}
