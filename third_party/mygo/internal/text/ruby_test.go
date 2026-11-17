package text

import (
	"reflect"
	"testing"
)

func TestRubySpanMetadataRoundTrip(t *testing.T) {
	spans := []Span{{End: 2, Leading: 5.375, Trailing: 11.375, RubyAscent: 47.33333, RubyScale: 1.5, NoBreak: 1}, {End: 5, Leading: 2, Trailing: 3, RubyAscent: 34.66667, RubyScale: 2, NoBreak: 1}}
	if got := DecodeSpans(EncodeSpans(spans)); !reflect.DeepEqual(got, spans) {
		t.Fatalf("ruby layout metadata lost: %#v", got)
	}
}

func TestRubyLineOnlyUsesAvailableEmSpace(t *testing.T) {
	font := &Font{Size: 24, TypoAscent: 21.36, TypoDescent: 2.64}
	previous := Line{Y: 0, Height: 32.4, Baseline: 25.344, Glyphs: []Glyph{{Y: 25.344, Font: font}}}
	line := Line{Y: 32.4, Height: 42, Ascent: 35, Baseline: 67.4, rubyExtra: 9.6, rubyScale: 1.5, Glyphs: []Glyph{{Y: 67.4}}}
	shift := tightenRubyLine(&line, previous, 24)
	if shift <= 0 || shift > 4.4 || line.Glyphs[0].Y != line.Baseline {
		t.Fatal("ruby used occupied space or detached glyph baseline", shift, line)
	}
	previous.Glyphs = []Glyph{{Object: true, Y: 25.344}}
	previous.Descent = 7.056
	line.rubyExtra = 9.6
	if shift := tightenRubyLine(&line, previous, 24); shift != 0 {
		t.Fatal("ruby overlaps preceding inline object", shift)
	}
}
