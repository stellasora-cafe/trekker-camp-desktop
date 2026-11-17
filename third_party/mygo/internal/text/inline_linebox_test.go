package text

import (
	"reflect"
	"testing"
)

func TestInlineLineBoxMetadataRoundTrip(t *testing.T) {
	spans := []Span{{End: 3, Family: "serif", LineAscent: 25.33333, LineDescent: 7.72917}, {End: 5, LineAscent: 30, LineDescent: 12, BaselineShift: 5, RubyAscent: 60, RubyScale: 1.5, Leading: 2, Trailing: 3, NoBreak: 1}, {End: 8}}
	if got := DecodeSpans(EncodeSpans(spans)); !reflect.DeepEqual(got, spans) {
		t.Fatalf("inline metrics or combined flags lost: %#v", got)
	}
}
