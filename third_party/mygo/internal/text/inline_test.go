package text

import (
	"slices"
	"testing"
)

func TestInlineNoBreakRanges(t *testing.T) {
	for _, s := range []string{"before 中文 ruby words after", "before אבג מילים after", "before ruby\nwords after", "before ru\u00adby words after"} {
		runes := []rune(s)
		spans := []Span{{End: 7}, {End: len(runes) - 6, NoBreak: 8}, {End: len(runes)}}
		key := EncodeSpans(spans)
		if !slices.Equal(DecodeSpans(key), spans) {
			t.Fatal("atomic range lost in layout key")
		}
		for _, width := range []float32{30, 90, 180} {
			l := Shared().Layout(Params{Text: s, Style: Style{Size: 20}, Width: width, Spans: key})
			for _, line := range l.Lines {
				if line.End > 7 && line.End < len(runes)-6 && runes[line.End-1] != '\n' && runes[line.End] != '\n' {
					t.Fatalf("%q at %v broke inside range at %d", s, width, line.End)
				}
			}
			if len(l.Runes) != len(runes) {
				t.Fatal("changed runes")
			}
		}
	}
}
