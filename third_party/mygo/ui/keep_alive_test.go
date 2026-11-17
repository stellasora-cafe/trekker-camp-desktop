package ui

import (
	"fmt"
	"testing"
)

func TestStateScopeSeparatesPagesAndOverlayIdentities(t *testing.T) {
	showFirst := true
	values := [2]*int{}
	var windowValue *int
	tt := NewTester(func(c *Context) {
		windowValue = Local(c.Root(), "counter", func() int { return 100 })
		Column(c).Fill().Children(func() {
			for i := range 2 {
				root := Box(c.Key(i)).Height(40)
				StateScope(c, root, func() {
					value := Local(StateRoot(c), "counter", func() int { return i })
					values[i] = value
					if i == 0 && !showFirst {
						return
					}
					Overlay(c, func() {
						if Local(StateRoot(c), "counter", func() int { return -1 }) != value {
							t.Error("overlay lost its page scope")
						}
						Box(c).Key("same-popup-key").Size(80, 25).Top(float32(i * 50)).Children(func() {
							v := Local(StateRoot(c), "counter", func() int { return -2 })
							Text(c, fmt.Sprint(*v))
						})
					})
				})
			}
		})
		if StateRoot(c) != c.Root() {
			t.Error("page scope leaked into window content")
		}
	}, 300, 300)
	defer tt.rt.close()
	tt.Frame()
	first, second := values[0], values[1]
	*second = 42
	showFirst = false
	tt.Frame()
	if values[0] != first || values[1] != second || *windowValue != 100 || *second != 42 {
		t.Fatal("page locals were shared or replaced")
	}
}

func TestStateScopeFollowsDeferredListRows(t *testing.T) {
	var list ListState
	late := false
	rows := 0
	tt := NewTester(func(c *Context) {
		root := Column(c.Key("page")).Fill()
		StateScope(c, root, func() {
			*Local(StateRoot(c), "page-name", func() string { return "page" }) = "page"
			List(c, &list, 200).Fill().Rows(func(row ListRow) {
				rows++
				if *Local(StateRoot(c), "page-name", func() string { return "wrong" }) != "page" {
					t.Fatal("layout-time row escaped its page scope")
				}
				if rows > 12 {
					late = true
				}
				Text(c, fmt.Sprint(row.Index)).Height(5)
			})
		})
	}, 300, 300)
	defer tt.rt.close()
	tt.Frame()
	if !late {
		t.Fatal("fixture did not exercise additional layout-time rows")
	}
}

func TestKeepAliveRetainsInputLocalsAndDisposesOnRemoval(t *testing.T) {
	active, present := true, true
	value := ""
	mounts, unmounts, builds := 0, 0, 0
	var saved *int
	tt := NewTester(func(c *Context) {
		if !present {
			return
		}
		KeepAlive(c, "page", active, func() {
			builds++
			state := Local(StateRoot(c), "value", func() int { return 7 })
			if saved == nil {
				saved = state
			} else if saved != state {
				t.Fatal("retained page remounted its Local")
			}
			input := TextInput(c.Key("input"), &value).Label("Editor").OnUnmount(func() { unmounts++ })
			Local(input, "mount", func() bool { mounts++; return true })
		})
	}, 300, 200)
	defer tt.rt.close()
	tt.Frame()
	if err := tt.Click("Editor"); err != nil {
		t.Fatal(err)
	}
	tt.Type("first")
	tt.Command("selectAll")
	tt.Type("second")
	active = false
	before := builds
	tt.Frame()
	tt.Frame()
	if builds != before || mounts != 1 || unmounts != 0 {
		t.Fatalf("deactivation changed lifetime: build=%d/%d mount=%d unmount=%d", builds, before, mounts, unmounts)
	}
	active = true
	tt.Frame()
	if !tt.Focused("Editor") {
		t.Fatal("returning page did not restore focus")
	}
	tt.Command("undo")
	if value != "first" || mounts != 1 || unmounts != 0 {
		t.Fatalf("editor history or lifetime lost: %q mount=%d unmount=%d", value, mounts, unmounts)
	}
	present = false
	tt.Frame()
	if unmounts != 1 {
		t.Fatalf("eviction unmounts=%d", unmounts)
	}
}

func TestKeepAliveRetainsPortalAndDeferredListLifetimes(t *testing.T) {
	active, present := true, true
	var list ListState
	var saved *int
	mounted, unmounted := 0, 0
	tt := NewTester(func(c *Context) {
		if !present {
			return
		}
		KeepAlive(c, "page", active, func() {
			List(c, &list, 100).Height(200).FillWidth().Rows(func(row ListRow) {
				Text(c, fmt.Sprint(row.Index)).Height(10)
			})
			Overlay(c, func() {
				popup := Box(c.Key("popup")).Size(80, 40).Label("Popup").OnUnmount(func() { unmounted++ })
				value := Local(popup, "value", func() int { mounted++; return 7 })
				if saved == nil {
					saved = value
				} else if saved != value {
					t.Fatal("portal remounted while its page was retained")
				}
			})
		})
	}, 300, 300)
	defer tt.rt.close()
	tt.Frame()
	active = false
	tt.Frame()
	tt.Frame()
	if mounted != 1 || unmounted != 0 {
		t.Fatalf("hidden portal mount=%d unmount=%d", mounted, unmounted)
	}
	active = true
	tt.Frame()
	if mounted != 1 || unmounted != 0 {
		t.Fatal("portal state did not survive reactivation")
	}
	present = false
	tt.Frame()
	tt.Frame()
	if unmounted != 1 {
		t.Fatalf("portal eviction unmount=%d", unmounted)
	}
}
