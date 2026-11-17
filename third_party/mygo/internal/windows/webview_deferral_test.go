//go:build windows && (amd64 || arm64)

package windows

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

// The fake interfaces use the SDK's vtable slots. Taking and releasing real
// COM references exercises the event's lifetime across the posted callback.
type eventTestCOM struct {
	vtbl         *[18]uintptr
	refs         int
	typ          string
	deferral     *eventTestCOM
	operation    *eventTestCOM
	failDeferral bool
	complete     int
	kind         int32
	state        int32
	cancel       uintptr
	handled      uintptr
	path         string
	stateHandler uintptr
}

var eventTestVtbl = func() (table [18]uintptr) {
	for i := range table {
		table[i] = syscall.NewCallback(func(this, a, b uintptr) uintptr {
			o := (*eventTestCOM)(native(this))
			switch i {
			case 1:
				o.refs++
				return uintptr(o.refs)
			case 2:
				o.refs--
				return uintptr(o.refs)
			}
			switch o.typ {
			case "deferral":
				if i == 3 {
					o.complete++
					return sOK
				}
			case "permission":
				switch i {
				case 3:
					*(*uintptr)(native(a)) = coTaskString(utf16z("https://example.com/page"))
					return sOK
				case 4:
					*(*int32)(native(a)) = o.kind
					return sOK
				case 7:
					o.state = int32(a)
					return sOK
				case 8:
					return o.getDeferral(a)
				}
			case "download":
				switch i {
				case 3:
					*(*uintptr)(native(a)) = uintptr(unsafe.Pointer(o.operation))
					o.operation.refs++
					return sOK
				case 5:
					o.cancel = a
					return sOK
				case 6:
					*(*uintptr)(native(a)) = coTaskString(utf16z(`C:\Downloads\sample.txt`))
					return sOK
				case 7:
					o.path = wstr(a)
					return sOK
				case 9:
					o.handled = a
					return sOK
				case 10:
					return o.getDeferral(a)
				}
			case "operation":
				switch i {
				case 7:
					o.stateHandler = a
					addRef(a)
					*(*int64)(native(b)) = 1
					return sOK
				case 9:
					*(*uintptr)(native(a)) = coTaskString(utf16z("https://example.com/sample.txt"))
					return sOK
				case 16:
					*(*int32)(native(a)) = o.state
					return sOK
				case 17:
					*(*int32)(native(a)) = 7
					return sOK
				}
			}
			panic("unexpected event COM method")
		})
	}
	return
}()

func (o *eventTestCOM) getDeferral(out uintptr) uintptr {
	if o.failDeferral {
		return eNotImpl
	}
	o.deferral.refs++
	*(*uintptr)(native(out)) = uintptr(unsafe.Pointer(o.deferral))
	return sOK
}

var eventTestObjects = map[*eventTestCOM]struct{}{}

func newEventTestCOM(t *testing.T, typ string) *eventTestCOM {
	o := &eventTestCOM{vtbl: &eventTestVtbl, refs: 1, typ: typ}
	eventTestObjects[o] = struct{}{} // COM stores addresses outside Go's pointer graph.
	t.Cleanup(func() { delete(eventTestObjects, o) })
	return o
}

type deferredEventHandler struct {
	platform.WindowHandler
	permission      func([]string, string) bool
	permissionAsync func([]string, string, func(bool))
	started         func(string, string) string
	finished        func(string, string, error)
}

func (h deferredEventHandler) PermissionRequested(kinds []string, origin string, respond func(bool)) {
	if h.permissionAsync != nil {
		h.permissionAsync(kinds, origin, respond)
	} else {
		respond(h.permission(kinds, origin))
	}
}
func (h deferredEventHandler) DownloadStarted(uri, name string) string { return h.started(uri, name) }
func (h deferredEventHandler) DownloadFinished(uri, path string, err error) {
	h.finished(uri, path, err)
}

func TestPermissionRequestedDeferral(t *testing.T) {
	for _, name := range []string{"allow", "deny", "closed", "close-in-dialog", "deferral-failed", "unhandled-kind"} {
		t.Run(name, func(t *testing.T) {
			d := newEventTestCOM(t, "deferral")
			a := newEventTestCOM(t, "permission")
			a.deferral, a.kind, a.failDeferral = d, 2, name == "deferral-failed"
			if name == "unhandled-kind" {
				a.kind = 99
			}
			b := &Backend{}
			w := &window{b: b}
			calls := 0
			w.h = deferredEventHandler{permission: func(kinds []string, origin string) bool {
				calls++
				if len(kinds) != 1 || kinds[0] != "camera" || origin != "https://example.com" {
					t.Errorf("permission = %v, %q", kinds, origin)
				}
				if name == "close-in-dialog" {
					w.closed = true
				}
				return name != "deny"
			}}
			w.permissionRequested(0, uintptr(unsafe.Pointer(a)))
			if calls != 0 || d.complete != 0 {
				t.Fatal("permission handler ran inside the COM callback")
			}
			release(uintptr(unsafe.Pointer(a))) // WebView2 returns from its event.
			w.closed = name == "closed"
			b.runPosted()
			wantCalls, wantComplete, wantState := 1, 1, int32(2)
			if name == "allow" {
				wantState = 1
			}
			if name == "closed" || name == "deferral-failed" || name == "unhandled-kind" {
				wantCalls = 0
			}
			if name == "deferral-failed" || name == "unhandled-kind" {
				wantComplete = 0
			}
			if name == "unhandled-kind" {
				wantState = 0
			}
			if calls != wantCalls || d.complete != wantComplete || a.state != wantState || a.refs != 0 || d.refs != 1 {
				t.Fatalf("calls=%d complete=%d state=%d refs=%d/%d; want %d/%d/%d/0/1", calls, d.complete, a.state, a.refs, d.refs, wantCalls, wantComplete, wantState)
			}
		})
	}
}

func TestDownloadStartingDeferral(t *testing.T) {
	for _, name := range []string{"save", "cancel", "closed", "close-in-dialog", "deferral-failed"} {
		t.Run(name, func(t *testing.T) {
			d, op := newEventTestCOM(t, "deferral"), newEventTestCOM(t, "operation")
			a := newEventTestCOM(t, "download")
			a.deferral, a.operation, a.failDeferral = d, op, name == "deferral-failed"
			path := filepath.Join(t.TempDir(), "sample.txt")
			if err := os.WriteFile(path, []byte("existing"), 0600); err != nil {
				t.Fatal(err)
			}
			b := &Backend{}
			w := &window{b: b}
			calls, finished := 0, 0
			w.h = deferredEventHandler{
				started: func(uri, suggested string) string {
					calls++
					if uri != "https://example.com/sample.txt" || suggested != "sample.txt" {
						t.Errorf("download = %q, %q", uri, suggested)
					}
					if name == "close-in-dialog" {
						w.closed = true
					}
					if name == "cancel" {
						return ""
					}
					return path
				},
				finished: func(uri, got string, err error) {
					finished++
					if got != path || uri != "https://example.com/sample.txt" || (op.state == 1 && (err == nil || !strings.Contains(err.Error(), "reason 7"))) || (op.state == 2 && err != nil) {
						t.Errorf("completion = %q, %q, %v", uri, got, err)
					}
				},
			}
			w.downloadStarting(0, uintptr(unsafe.Pointer(a)))
			if calls != 0 || d.complete != 0 || a.cancel != 1 || a.handled != 1 {
				t.Fatal("download handler ran or download proceeded inside the COM callback")
			}
			release(uintptr(unsafe.Pointer(a)))
			w.closed = name == "closed"
			b.runPosted()
			wantCalls, wantComplete, wantCancel := 1, 1, uintptr(1)
			if name == "save" {
				wantCancel = 0
			}
			if name == "closed" || name == "deferral-failed" {
				wantCalls = 0
			}
			if name == "deferral-failed" {
				wantComplete = 0
			}
			if calls != wantCalls || d.complete != wantComplete || a.cancel != wantCancel || a.refs != 0 || d.refs != 1 || op.refs != 1 {
				t.Fatalf("calls=%d complete=%d cancel=%d refs=%d/%d/%d", calls, d.complete, a.cancel, a.refs, d.refs, op.refs)
			}
			if name != "save" {
				if data, err := os.ReadFile(path); err != nil || string(data) != "existing" {
					t.Fatalf("canceled download changed the existing file: %q, %v", data, err)
				}
				return
			}
			if a.path != path || op.stateHandler == 0 {
				t.Fatalf("saved path=%q, state handler=%#x", a.path, op.stateHandler)
			}
			defer release(op.stateHandler)
			for _, state := range []int32{1, 2} {
				op.state = state
				before := finished
				comCall(op.stateHandler, 3, uintptr(unsafe.Pointer(op)), 0)
				if finished != before {
					t.Fatal("completion handler ran inside the COM callback")
				}
				b.runPosted()
				if finished != before+1 {
					t.Fatal("completion handler was not dispatched")
				}
			}
			comCall(op.stateHandler, 3, uintptr(unsafe.Pointer(op)), 0)
			w.closed = true
			b.runPosted()
			if finished != 2 {
				t.Fatal("completion handler ran after the window closed")
			}
		})
	}
}

func TestPermissionRequestedAsyncDeferral(t *testing.T) {
	for _, name := range []string{"allow", "deny", "close-window", "close-embedded", "close-before-dispatch"} {
		t.Run(name, func(t *testing.T) {
			d, a := newEventTestCOM(t, "deferral"), newEventTestCOM(t, "permission")
			a.deferral, a.kind = d, 4
			b := &Backend{}
			w := &window{b: b, opts: &platform.WindowOptions{}}
			var respond func(bool)
			w.h = deferredEventHandler{permissionAsync: func(kinds []string, origin string, answer func(bool)) {
				respond = answer
			}}
			w.permissionRequested(0, uintptr(unsafe.Pointer(a)))
			release(uintptr(unsafe.Pointer(a)))
			if name == "close-before-dispatch" {
				w.cleanup()
			}
			b.runPosted()
			if name == "close-before-dispatch" {
				if respond != nil {
					t.Fatal("closed window opened a permission prompt")
				}
			} else {
				if respond == nil || d.complete != 0 || a.refs != 1 || d.refs != 2 || len(w.permissions) != 1 {
					t.Fatal("the native request was not retained while awaiting the user")
				}
				switch name {
				case "close-window":
					w.cleanup()
				case "close-embedded":
					w.host = &window{}
					w.closeWebView()
				default:
					respond(name == "allow")
				}
				respond(true)
				respond(false)
			}
			want := int32(2)
			if name == "allow" {
				want = 1
			}
			if a.state != want || d.complete != 1 || a.refs != 0 || d.refs != 1 || len(w.permissions) != 0 {
				t.Fatalf("state=%d complete=%d refs=%d/%d pending=%d", a.state, d.complete, a.refs, d.refs, len(w.permissions))
			}
		})
	}
}
