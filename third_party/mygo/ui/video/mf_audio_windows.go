//go:build windows && (amd64 || arm64)

package video

import (
	"encoding/binary"
	"math"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// This synchronous, in-place IMFTransform observes decoded audio without
// changing the sample sent to the audio renderer. Media Engine's volume/mute
// happen downstream, as with a Web Audio analyser connected before its gain.
type mfAudioTap struct {
	ptr                    uintptr
	id                     uint64
	refs                   atomic.Int32
	mu                     sync.Mutex
	input, output, pending uintptr
	format                 audioPCMFormat
	chunks                 []audioPCMChunk
}

type audioPCMFormat struct {
	rate, channels, bits int
	floating             bool
}
type audioPCMChunk struct {
	start   time.Duration
	samples []float32
}

var (
	audioTapOnce          sync.Once
	audioTapVTable        [26]uintptr
	audioTaps             sync.Map
	lastAudioTap          atomic.Uint64
	procMFCreateMediaType = mfplat.NewProc("MFCreateMediaType")
	iidIMFTransform       = guid{0xbf94c121, 0x5b05, 0x4e6f, [8]byte{0x80, 0x00, 0xba, 0x59, 0x89, 0x61, 0x41, 0x4d}}
	mtMajor               = guid{0x48eba18e, 0xf8c9, 0x4687, [8]byte{0xbf, 0x11, 0x0a, 0x74, 0xc9, 0xf9, 0x6a, 0x8f}}
	mtSubtype             = guid{0xf7e34c9a, 0x42e8, 0x4714, [8]byte{0xb7, 0x4b, 0xcb, 0x29, 0xd7, 0x2c, 0x35, 0xe5}}
	mtChannels            = guid{0x37e48bf5, 0x645e, 0x4c5b, [8]byte{0x89, 0xde, 0xad, 0xa9, 0xe2, 0x9b, 0x69, 0x6a}}
	mtSampleRate          = guid{0x5faeeae7, 0x0290, 0x4c31, [8]byte{0x9e, 0x8a, 0xc5, 0x34, 0xf6, 0x8d, 0x9d, 0xba}}
	mtSampleBits          = guid{0xf2deb57f, 0x40fa, 0x4764, [8]byte{0xaa, 0x33, 0xed, 0x4f, 0x2d, 0x1f, 0xf6, 0x69}}
	majorAudio            = guid{0x73647561, 0, 0x10, [8]byte{0x80, 0, 0, 0xaa, 0, 0x38, 0x9b, 0x71}}
	subtypePCM            = guid{1, 0, 0x10, [8]byte{0x80, 0, 0, 0xaa, 0, 0x38, 0x9b, 0x71}}
	subtypeFloat          = guid{3, 0, 0x10, [8]byte{0x80, 0, 0, 0xaa, 0, 0x38, 0x9b, 0x71}}
)

const (
	mfTypeNotSet   = 0xc00d6d60
	mfNeedInput    = 0xc00d6d72
	mfInvalidType  = 0xc00d36b4
	mfNotAccepting = 0xc00d36b0
	eInvalidArg    = 0x80070057
	eNotImpl       = 0x80004001
)

func (e *mfEngine) installAudioTap() {
	if e.ex == 0 || procMFCreateMediaType.Find() != nil {
		return
	}
	audioTapOnce.Do(func() {
		audioTapVTable = [26]uintptr{
			syscall.NewCallback(tapQuery), syscall.NewCallback(tapAddRef), syscall.NewCallback(tapRelease),
			syscall.NewCallback(tapLimits), syscall.NewCallback(tapCount), syscall.NewCallback(tapIDs),
			syscall.NewCallback(tapInputInfo), syscall.NewCallback(tapOutputInfo),
			syscall.NewCallback(tapAttributes), syscall.NewCallback(tapStreamAttributes), syscall.NewCallback(tapStreamAttributes),
			syscall.NewCallback(tapDelete), syscall.NewCallback(tapAdd),
			syscall.NewCallback(tapInputAvailable), syscall.NewCallback(tapOutputAvailable),
			syscall.NewCallback(tapSetInput), syscall.NewCallback(tapSetOutput),
			syscall.NewCallback(tapGetInput), syscall.NewCallback(tapGetOutput),
			syscall.NewCallback(tapInputStatus), syscall.NewCallback(tapOutputStatus),
			syscall.NewCallback(tapBounds), syscall.NewCallback(tapEvent), syscall.NewCallback(tapMessage),
			syscall.NewCallback(tapProcessInput), syscall.NewCallback(tapProcessOutput),
		}
	})
	p, _, _ := procCoTaskMemAlloc.Call(16)
	if p == 0 {
		return
	}
	t := &mfAudioTap{ptr: p, id: lastAudioTap.Add(1)}
	t.refs.Store(1)
	*(*uintptr)(cptr(p)) = uintptr(unsafe.Pointer(&audioTapVTable))
	*(*uint64)(cptr(p + 8)) = t.id
	audioTaps.Store(t.id, t)
	if failed(call(e.ex, 61, p, 1)) {
		tapRelease(p)
		return
	}
	e.audioTap = t
}

func (e *mfEngine) releaseAudioTap() {
	if e.audioTap != nil {
		tapRelease(e.audioTap.ptr)
		e.audioTap = nil
	}
}
func tapOf(p uintptr) *mfAudioTap {
	v, _ := audioTaps.Load(*(*uint64)(cptr(p + 8)))
	return v.(*mfAudioTap)
}
func tapQuery(p, iid, out uintptr) uintptr {
	*(*uintptr)(cptr(out)) = 0
	g := *(*guid)(cptr(iid))
	if g != iidIUnknown && g != iidIMFTransform {
		return 0x80004002
	}
	*(*uintptr)(cptr(out)) = p
	tapAddRef(p)
	return 0
}
func tapAddRef(p uintptr) uintptr { return uintptr(tapOf(p).refs.Add(1)) }
func tapRelease(p uintptr) uintptr {
	t := tapOf(p)
	n := t.refs.Add(-1)
	if n == 0 {
		audioTaps.Delete(t.id)
		free(&t.pending)
		free(&t.input)
		free(&t.output)
		procCoTaskMemFree.Call(p)
	}
	return uintptr(n)
}
func tapLimits(p, a, b, c, d uintptr) uintptr {
	for _, v := range []uintptr{a, b, c, d} {
		*(*uint32)(cptr(v)) = 1
	}
	return 0
}
func tapCount(p, a, b uintptr) uintptr     { *(*uint32)(cptr(a)) = 1; *(*uint32)(cptr(b)) = 1; return 0 }
func tapIDs(p, a, b, c, d uintptr) uintptr { return eNotImpl }

type tapInputStreamInfo struct {
	latency                           int64
	flags, size, lookahead, alignment uint32
}
type tapOutputStreamInfo struct{ flags, size, alignment uint32 }
type tapOutputBuffer struct {
	stream uint32
	sample uintptr
	status uint32
	events uintptr
}

func tapInputInfo(p, stream, out uintptr) uintptr {
	if stream != 0 {
		return eInvalidArg
	}
	*(*tapInputStreamInfo)(cptr(out)) = tapInputStreamInfo{flags: 1 | 2 | 8 | 0x800}
	return 0
}
func tapOutputInfo(p, stream, out uintptr) uintptr {
	if stream != 0 {
		return eInvalidArg
	}
	*(*tapOutputStreamInfo)(cptr(out)) = tapOutputStreamInfo{flags: 1 | 2 | 0x100}
	return 0
}
func tapAttributes(p, out uintptr) uintptr               { *(*uintptr)(cptr(out)) = 0; return eNotImpl }
func tapStreamAttributes(p, stream, out uintptr) uintptr { return tapAttributes(p, out) }
func tapDelete(p, stream uintptr) uintptr                { return eNotImpl }
func tapAdd(p, count, streams uintptr) uintptr           { return eNotImpl }
func tapInputAvailable(p, stream, index, out uintptr) uintptr {
	return tapAvailable(p, stream, index, out, false)
}
func tapOutputAvailable(p, stream, index, out uintptr) uintptr {
	return tapAvailable(p, stream, index, out, true)
}
func tapAvailable(p, stream, index, out uintptr, output bool) uintptr {
	*(*uintptr)(cptr(out)) = 0
	if stream != 0 {
		return eInvalidArg
	}
	t := tapOf(p)
	t.mu.Lock()
	defer t.mu.Unlock()
	other := t.output
	if output {
		other = t.input
	}
	if other != 0 {
		if index != 0 {
			return 0xc00d36b9
		}
		call(other, 1)
		*(*uintptr)(cptr(out)) = other
		return 0
	}
	if index > 1 {
		return 0xc00d36b9
	}
	var mt uintptr
	hr, _, _ := procMFCreateMediaType.Call(uintptr(unsafe.Pointer(&mt)))
	if failed(hr) {
		return hr
	}
	call(mt, 24, uintptr(unsafe.Pointer(&mtMajor)), uintptr(unsafe.Pointer(&majorAudio)))
	subtype := subtypeFloat
	if index == 1 {
		subtype = subtypePCM
	}
	call(mt, 24, uintptr(unsafe.Pointer(&mtSubtype)), uintptr(unsafe.Pointer(&subtype)))
	*(*uintptr)(cptr(out)) = mt
	return 0
}
func pcmFormat(mt uintptr) (f audioPCMFormat, ok bool) {
	var major, subtype guid
	if failed(call(mt, 10, uintptr(unsafe.Pointer(&mtMajor)), uintptr(unsafe.Pointer(&major)))) || major != majorAudio {
		return
	}
	if failed(call(mt, 10, uintptr(unsafe.Pointer(&mtSubtype)), uintptr(unsafe.Pointer(&subtype)))) || (subtype != subtypePCM && subtype != subtypeFloat) {
		return
	}
	attrs := []*guid{&mtChannels, &mtSampleRate, &mtSampleBits}
	v := [3]uint32{}
	for i, g := range attrs {
		if failed(call(mt, 7, uintptr(unsafe.Pointer(g)), uintptr(unsafe.Pointer(&v[i])))) {
			return
		}
	}
	f = audioPCMFormat{channels: int(v[0]), rate: int(v[1]), bits: int(v[2]), floating: subtype == subtypeFloat}
	ok = f.channels > 0 && f.rate > 0 && ((f.floating && f.bits == 32) || (!f.floating && (f.bits == 8 || f.bits == 16 || f.bits == 24 || f.bits == 32)))
	return
}
func tapSetInput(p, stream, mt, flags uintptr) uintptr {
	return tapSetType(p, stream, mt, flags, false)
}
func tapSetOutput(p, stream, mt, flags uintptr) uintptr {
	return tapSetType(p, stream, mt, flags, true)
}
func tapSetType(p, stream, mt, flags uintptr, output bool) uintptr {
	if stream != 0 || flags > 1 {
		return eInvalidArg
	}
	t := tapOf(p)
	t.mu.Lock()
	defer t.mu.Unlock()
	var f audioPCMFormat
	if mt != 0 {
		var ok bool
		f, ok = pcmFormat(mt)
		if !ok {
			return mfInvalidType
		}
		other := t.output
		if output {
			other = t.input
		}
		if other != 0 {
			of, _ := pcmFormat(other)
			if f != of {
				return mfInvalidType
			}
		}
	}
	if flags == 1 {
		return 0
	}
	if t.pending != 0 {
		return mfNotAccepting
	}
	dest := &t.input
	if output {
		dest = &t.output
	}
	if mt != 0 {
		call(mt, 1)
	}
	free(dest)
	*dest = mt
	if !output {
		t.format = f
		t.chunks = nil
	}
	return 0
}
func tapGetInput(p, stream, out uintptr) uintptr  { return tapCurrent(p, stream, out, false) }
func tapGetOutput(p, stream, out uintptr) uintptr { return tapCurrent(p, stream, out, true) }
func tapCurrent(p, stream, out uintptr, output bool) uintptr {
	*(*uintptr)(cptr(out)) = 0
	if stream != 0 {
		return eInvalidArg
	}
	t := tapOf(p)
	t.mu.Lock()
	defer t.mu.Unlock()
	mt := t.input
	if output {
		mt = t.output
	}
	if mt == 0 {
		return mfTypeNotSet
	}
	call(mt, 1)
	*(*uintptr)(cptr(out)) = mt
	return 0
}
func tapInputStatus(p, stream, out uintptr) uintptr {
	if stream != 0 {
		return eInvalidArg
	}
	t := tapOf(p)
	t.mu.Lock()
	defer t.mu.Unlock()
	*(*uint32)(cptr(out)) = 0
	if t.pending == 0 {
		*(*uint32)(cptr(out)) = 1
	}
	return 0
}
func tapOutputStatus(p, out uintptr) uintptr {
	t := tapOf(p)
	t.mu.Lock()
	defer t.mu.Unlock()
	*(*uint32)(cptr(out)) = 0
	if t.pending != 0 {
		*(*uint32)(cptr(out)) = 1
	}
	return 0
}
func tapBounds(p, lower, upper uintptr) uintptr { return eNotImpl }
func tapEvent(p, stream, event uintptr) uintptr { return eNotImpl }
func tapMessage(p, message, param uintptr) uintptr {
	t := tapOf(p)
	t.mu.Lock()
	defer t.mu.Unlock()
	if message == 0 {
		free(&t.pending)
		t.chunks = nil
	}
	return 0
}
func tapProcessInput(p, stream, sample, flags uintptr) uintptr {
	if stream != 0 || sample == 0 || flags != 0 {
		return eInvalidArg
	}
	t := tapOf(p)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.input == 0 || t.output == 0 {
		return mfTypeNotSet
	}
	if t.pending != 0 {
		return mfNotAccepting
	}
	call(sample, 1)
	t.pending = sample
	t.capture(sample)
	return 0
}
func tapProcessOutput(p, flags, count, buffers, status uintptr) uintptr {
	if flags != 0 || count != 1 || buffers == 0 || status == 0 {
		return eInvalidArg
	}
	t := tapOf(p)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pending == 0 {
		return mfNeedInput
	}
	b := (*tapOutputBuffer)(cptr(buffers))
	b.sample = t.pending
	b.status = 0
	b.events = 0
	t.pending = 0
	*(*uint32)(cptr(status)) = 0
	return 0
}

func (t *mfAudioTap) capture(sample uintptr) {
	var timestamp int64
	if failed(call(sample, 35, uintptr(unsafe.Pointer(&timestamp)))) {
		return
	}
	var buffer uintptr
	if failed(call(sample, 41, uintptr(unsafe.Pointer(&buffer)))) || buffer == 0 {
		return
	}
	defer free(&buffer)
	var data unsafe.Pointer
	var length uint32
	if failed(call(buffer, 3, uintptr(unsafe.Pointer(&data)), 0, uintptr(unsafe.Pointer(&length)))) {
		return
	}
	pcm := decodeAudioPCM(unsafe.Slice((*byte)(data), int(length)), t.format)
	call(buffer, 4)
	if len(pcm) == 0 {
		return
	}
	start := time.Duration(timestamp) * 100
	t.chunks = append(t.chunks, audioPCMChunk{start, pcm})
	// Retain the analyser window plus the renderer's decode-ahead. Never retain
	// an entire downloaded file; timestamped samples are discarded as it plays.
	cutoff := start - 2*time.Second
	first := 0
	for first < len(t.chunks)-1 && t.chunks[first].start < cutoff {
		first++
	}
	if first > 0 {
		copy(t.chunks, t.chunks[first:])
		clear(t.chunks[len(t.chunks)-first:])
		t.chunks = t.chunks[:len(t.chunks)-first]
	}
}

func decodeAudioPCM(data []byte, f audioPCMFormat) []float32 {
	stride := f.channels * f.bits / 8
	if stride <= 0 {
		return nil
	}
	result := make([]float32, len(data)/stride)
	for i := range result {
		var sum float32
		for ch := 0; ch < f.channels; ch++ {
			p := data[i*stride+ch*f.bits/8:]
			var sample float32
			switch {
			case f.floating:
				sample = math.Float32frombits(binary.LittleEndian.Uint32(p))
			case f.bits == 8:
				sample = float32(int(p[0])-128) / 128
			case f.bits == 16:
				sample = float32(int16(binary.LittleEndian.Uint16(p))) / 32768
			case f.bits == 24:
				sample = float32(int32(uint32(p[0])<<8|uint32(p[1])<<16|uint32(p[2])<<24)) / 2147483648
			case f.bits == 32:
				sample = float32(int32(binary.LittleEndian.Uint32(p))) / 2147483648
			}
			// AnalyserNode uses speaker down-mixing. Standard 5.1 excludes
			// LFE and gives center/surround channels their defined weights.
			weight := float32(1) / float32(f.channels)
			if f.channels == 6 {
				switch ch {
				case 0, 1:
					weight = float32(math.Sqrt(.5))
				case 2:
					weight = 1
				case 3:
					weight = 0
				case 4, 5:
					weight = .5
				}
			}
			sum += sample * weight
		}
		result[i] = sum
	}
	return result
}

func (e *mfEngine) AudioSamples(position time.Duration, dst []float32) (int, bool) {
	if e.audioTap == nil {
		return 0, false
	}
	t := e.audioTap
	t.mu.Lock()
	defer t.mu.Unlock()
	rate := t.format.rate
	if rate == 0 {
		return 0, false
	}
	clear(dst)
	start := int64(math.Round(position.Seconds()*float64(rate))) - int64(len(dst))
	for _, chunk := range t.chunks {
		at := int64(math.Round(chunk.start.Seconds()*float64(rate))) - start
		from := max(0, -at)
		to := min(int64(len(chunk.samples)), int64(len(dst))-at)
		if to > from {
			copy(dst[at+from:at+to], chunk.samples[from:to])
		}
	}
	return rate, true
}
