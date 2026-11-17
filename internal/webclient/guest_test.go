// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"context"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestGuestWelcomeFixedCardAndSeparateActions(t *testing.T) {
	if os.Getenv("CAMP_GUEST_SNAPSHOTS") != "" {
		for _, path := range permissionFontCandidates("") {
			if data, err := os.ReadFile(path); err == nil && ui.RegisterFont(data, "MiSans") == nil {
				break
			}
		}
	}
	for _, size := range [][2]int{{420, 460}} {
		p := newGuestPanel(context.Background(), "http://localhost:3000", t.TempDir(), guestCallbacks{})
		defer p.Close()
		var opened string
		p.callbacks.OpenTool = func(path string) { opened = path }
		tester := ui.NewTester(p.View, size[0], size[1])
		for _, point := range [][2]int{{0, 0}, {size[0] / 2, 3}, {3, size[1] / 2}, {size[0] - 1, size[1] - 1}} {
			if alpha := tester.Image().RGBAAt(point[0], point[1]).A; alpha != 255 {
				t.Fatalf("%v outer pixel %v must be opaque, alpha=%d", size, point, alpha)
			}
		}
		for _, name := range []string{"最小化", "最大化", "关闭窗口", "更多选项"} {
			bounds, ok := tester.Find(name)
			if !ok || bounds.X < 0 || bounds.Y < 0 || bounds.X+bounds.W > float32(size[0]) || bounds.Y+bounds.H > float32(size[1]) {
				t.Fatalf("%v missing/clipped %s: %+v", size, name, bounds)
			}
		}
		register, _ := tester.Find("注册")
		// Sample the solid fill, away from the rounded corners and label.
		// The decorative gradients must not tint the foreground controls.
		if got := tester.Image().RGBAAt(int(register.X+35), int(register.Y+register.H/2)); got != (color.RGBA{R: 0, G: 169, B: 181, A: 255}) {
			t.Fatalf("welcome action color changed by its backdrop: %v", got)
		}
		login, _ := tester.Find("登录")
		toolbox, _ := tester.Find("不登录使用星塔工具箱（待开放）")
		if register.H < 41.9 || login.H < 41.9 || toolbox.H < 41.9 || login.Y < register.Y+register.H+9.9 || toolbox.Y < login.Y+login.H+9.9 {
			t.Fatalf("%v buttons squeezed/overlapping: %+v %+v %+v", size, register, login, toolbox)
		}
		if toolbox.Y+toolbox.H > float32(size[1])-8 {
			t.Fatalf("normal size hides toolbox: %+v", toolbox)
		}
		if size == [2]int{420, 460} {
			if dir := os.Getenv("CAMP_GUEST_SNAPSHOTS"); dir != "" {
				_ = os.MkdirAll(dir, 0700)
				f, err := os.Create(filepath.Join(dir, "native-guest.png"))
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(f, tester.Image()); err != nil {
					t.Fatal(err)
				}
				_ = f.Close()
			}
			if err := tester.Click("注册"); err != nil || opened != "/desktop-auth?mode=signup" {
				t.Fatalf("signup action = %q, %v", opened, err)
			}
			if err := tester.Click("登录"); err != nil {
				t.Fatal(err)
			}
			if opened != "/desktop-auth?mode=signin" {
				t.Fatal("login must open a separate web window without replacing the welcome card")
			}
			if _, ok := tester.Find("用户名"); ok {
				t.Fatal("login form must not appear inside welcome card")
			}
		}
	}
}

func TestGuestMetaCacheIsOriginScoped(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "guest-meta.json"), []byte(`{"Origin":"https://one.test","Name":"saved","Description":"<b>cached</b>"}`), 0600)
	one := newGuestPanel(context.Background(), "https://one.test", dir, guestCallbacks{})
	defer one.Close()
	two := newGuestPanel(context.Background(), "https://two.test", dir, guestCallbacks{})
	defer two.Close()
	if one.name != "saved" || one.description != "cached" || two.name == "saved" {
		t.Fatal("metadata cache crossed site boundary")
	}
}

func TestGuestLoadFailureKeepsActionsAndLocalMenusAvailable(t *testing.T) {
	p := newGuestPanel(context.Background(), "http://localhost:3000", t.TempDir(), guestCallbacks{})
	defer p.Close()
	p.loadError = "加载失败，请检查网络后重试。"
	var opened string
	var about bool
	p.callbacks.OpenTool = func(path string) { opened = path }
	p.callbacks.AboutCamp = func() { about = true }
	tester := ui.NewTester(p.View, 420, 460)
	if err := tester.Click("登录"); err != nil || opened != "/desktop-auth?mode=signin" {
		t.Fatal("failed load must allow retry", err)
	}
	if err := tester.Click("更多选项"); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem("关于营地"); err != nil || !about {
		t.Fatal("failed load disabled local menus", err)
	}
}

func TestGuestToolsMenuUsesCampControlsAndRestoresFocus(t *testing.T) {
	p := newGuestPanel(context.Background(), "http://localhost:3000", t.TempDir(), guestCallbacks{})
	defer p.Close()
	var opened string
	p.callbacks.OpenTool = func(path string) { opened = path }
	tester := ui.NewTester(p.View, 420, 460)
	if err := tester.Click("更多选项"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("服务器信息") {
		t.Fatal("tools menu must be rendered inside the camp window")
	}
	if err := tester.ChooseMenuItem("服务器信息"); err != nil || opened != "/about" {
		t.Fatalf("tool selection = %q, %v", opened, err)
	}
	if !tester.Focused("更多选项") {
		t.Fatal("menu must restore focus to its trigger")
	}
	if err := tester.Click("更多选项"); err != nil {
		t.Fatal(err)
	}
	tester.Key(0, ui.KeyEscape)
	if tester.HasText("服务器信息") || !tester.Focused("更多选项") {
		t.Fatal("Escape must dismiss the menu and restore focus")
	}
}

func TestGuestCompactDescriptionKeepsButtonsVisible(t *testing.T) {
	p := newGuestPanel(context.Background(), "http://localhost:3000", t.TempDir(), guestCallbacks{})
	defer p.Close()
	p.description = "基于 Misskey 的星塔旅人非官方社区，不会启用联邦。Q 群：1131187918。欢迎各位魔王来到旅人营地，分享故事和旅途见闻。爱与希望，就在路上。"
	tester := ui.NewTester(p.View, 420, 460)
	for _, label := range []string{"注册", "登录", "不登录使用星塔工具箱（待开放）"} {
		bounds, ok := tester.Find(label)
		if !ok || bounds.H < 41.9 || bounds.Y+bounds.H > 436 {
			t.Fatalf("long description clips %s: %+v", label, bounds)
		}
	}
}
