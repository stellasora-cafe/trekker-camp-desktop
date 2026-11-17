//go:build windows && (amd64 || arm64)

package video

import (
	"github.com/egoist/mygo/internal/media"
	"math"
	"time"
	"unsafe"
)

func (e *mfEngine) Buffered() []media.TimeRange {
	var ranges uintptr
	if e.engine == 0 || failed(call(e.engine, 11, uintptr(unsafe.Pointer(&ranges)))) || ranges == 0 {
		return nil
	}
	defer free(&ranges)
	count := int(call(ranges, 3))
	result := make([]media.TimeRange, 0, count)
	for i := 0; i < count; i++ {
		var start, end float64
		if failed(call(ranges, 4, uintptr(i), uintptr(unsafe.Pointer(&start)))) || failed(call(ranges, 5, uintptr(i), uintptr(unsafe.Pointer(&end)))) {
			continue
		}
		if !math.IsNaN(start) && !math.IsInf(start, 0) && !math.IsNaN(end) && !math.IsInf(end, 0) && end >= start {
			result = append(result, media.TimeRange{Start: time.Duration(start * float64(time.Second)), End: time.Duration(end * float64(time.Second))})
		}
	}
	return result
}
