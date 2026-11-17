package ui

import (
	"math"
	"testing"
)

func TestInputLineBoxWrappingCompositionAndReset(t *testing.T) {
	value := "one\n\n最后\n"
	var lines TextLines
	exact := true
	tt := coreNewTester(func(c *context) {
		in := coreTextAreaBase(c, &value).Width(120).Padding(0).Lines(1, 100).FontSize(24).TrackLines(&lines)
		if exact {
			in.InputLineBox(12, 4)
		}
	}, 200, 300)
	tt.Frame()
	if lines.Count() != 4 || lines.Height() != 64 {
		t.Fatalf("empty lines=%d height=%v", lines.Count(), lines.Height())
	}
	st := textAreaState(tt)
	if st.editor.area.line.Ascent != 12 || st.editor.area.line.Descent != 4 {
		t.Fatal(st.editor.area.line)
	}
	tt.ClickAt(10, 10)
	tt.Key(Ctrl, KeyEnd)
	tt.Compose("正在组合中文", 3)
	if math.Mod(float64(lines.Height()), 16) != 0 {
		t.Fatalf("composition height=%v", lines.Height())
	}
	tt.Type("wrap wrap wrap wrap wrap wrap")
	if len(lines.Rows(3)) < 2 {
		t.Fatal("text did not wrap")
	}
	for i := 0; i < lines.Count(); i++ {
		if math.Mod(float64(lines.Top(i)), 16) != 0 {
			t.Fatal("wrapped baseline", lines.Top(i))
		}
	}
	exact = false
	tt.Frame()
	if st.editor.area.line.Height == 16 || st.editor.area.line.Ascent == 12 {
		t.Fatal("line box leaked to a later build")
	}
}
