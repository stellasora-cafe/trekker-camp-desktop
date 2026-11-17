package text

import (
	"encoding/binary"
	"unsafe"
)

// OpenType OS/2 typographic metrics describe the ideographic em box used by
// ruby, independently of the larger hhea/Windows clipping metrics.
func readRubyFontMetrics(f *Font, face uintptr) {
	var data unsafe.Pointer
	var length uint32
	var context uintptr
	var exists int32
	const tryGetFontTable = 12
	const releaseFontTable = 13
	const os2Tag = uintptr('O') | uintptr('S')<<8 | uintptr('/')<<16 | uintptr('2')<<24
	if failed(call(face, tryGetFontTable, os2Tag, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&length)), uintptr(unsafe.Pointer(&context)), uintptr(unsafe.Pointer(&exists)))) || exists == 0 {
		return
	}
	defer call(face, releaseFontTable, context)
	if length < 72 || data == nil {
		return
	}
	table := unsafe.Slice((*byte)(data), int(length))
	a := float32(int16(binary.BigEndian.Uint16(table[68:70])))
	d := -float32(int16(binary.BigEndian.Uint16(table[70:72])))
	if a > 0 && d >= 0 {
		f.TypoAscent, f.TypoDescent = f.Size*a/(a+d), f.Size*d/(a+d)
	}
}
