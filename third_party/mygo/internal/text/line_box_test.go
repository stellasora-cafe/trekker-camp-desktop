package text

import (
	"math"
	"reflect"
	"testing"
	"unicode/utf8"
)

func TestExactLineBoxMetricsAndFontFallback(t *testing.T) {
	for _, content := range []string{"MiSans text\nsecond line", "中文😀\n第二行", "a\n\nb"} {
		spans := []Span{{End: utf8.RuneCountInString(content), LineAscent: 13.333333, LineDescent: 4.125, ExactLineBox: true}}
		p := Params{Text: content, Style: Style{Family: "MiSans, sans-serif", Size: 12.936, LineHeight: 1.35}, Spans: EncodeSpans(spans)}
		layout := Shared().Layout(p)
		for i, line := range layout.Lines {
			if math.Abs(float64(line.Height-17.458333)) > .0001 || math.Abs(float64(line.Baseline-line.Y-13.333333)) > .0001 {
				t.Fatalf("%q line %d: %+v", content, i, line)
			}
		}
		if !reflect.DeepEqual(layout.Runes, []rune(content)) {
			t.Fatal("line metrics changed text or selection offsets")
		}
		for _, line := range layout.Lines {
			for _, glyph := range line.Glyphs {
				if glyph.Size != 12.936 {
					t.Fatal("line box scaled glyphs", glyph.Size)
				}
			}
		}
	}
}

func TestExactLineBoxPreservesUnspecifiedStrutsAndReserve(t *testing.T) {
	s := Shared()
	p := Params{Text: "small LARGE", Style: Style{Size: 18}}
	plain := s.Layout(p)
	p.Spans = EncodeSpans([]Span{{End: 11, ExactLineBox: true}})
	if got := s.Layout(p); got.Height != plain.Height {
		t.Fatal("flag without metrics changed default", got.Height, plain.Height)
	}
	p.Spans = EncodeSpans([]Span{{End: 6, LineAscent: 5, LineDescent: 2, ExactLineBox: true}, {End: 11, Size: 32}})
	mixed := s.Layout(p)
	if mixed.Height <= plain.Height {
		t.Fatal("unmarked larger font did not reserve its own extents")
	}
	p.Spans = EncodeSpans([]Span{{End: 11, LineAscent: 50, LineDescent: 20}})
	if got := s.Layout(p); got.Height < 70 {
		t.Fatal("existing reserve behavior changed", got.Height)
	}
	spans := []Span{{End: 4, LineAscent: 13, LineDescent: 4, ExactLineBox: true}, {End: 11, LineAscent: 20, LineDescent: 5, NoBreak: 2, Leading: 3, RubyAscent: 30, RubyScale: 1.5}}
	if got := DecodeSpans(EncodeSpans(spans)); !reflect.DeepEqual(got, spans) {
		t.Fatal("cache encoding lost line box/ruby metadata", got)
	}
}
