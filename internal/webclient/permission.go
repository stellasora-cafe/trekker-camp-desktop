// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type permissionDecision struct {
	request mygo.PermissionRequest
	respond func(bool)
	once    sync.Once
}

func (p *permissionDecision) resolve(allowed bool) { p.once.Do(func() { p.respond(allowed) }) }

func (c *client) requestPermission(req mygo.PermissionRequest, respond func(bool)) {
	if _, ok := permissionLabel(c.origin, req); !ok || c.ctx.Err() != nil || c.window.IsDestroyed() {
		respond(false)
		return
	}
	c.permissionQueue = append(c.permissionQueue, &permissionDecision{request: req, respond: respond})
	c.showPermissionPrompt()
}

// Native child modality disables the parent, so camera and microphone requests
// must share a queue instead of opening overlapping modal windows.
func (c *client) showPermissionPrompt() {
	if c.ctx.Err() != nil || c.window.IsDestroyed() {
		c.closePermissionPrompts()
		return
	}
	if c.inTray.Load() || c.permissionWindow != nil || len(c.permissionQueue) == 0 {
		return
	}
	pending := c.permissionQueue[0]
	model := permissionContent(pending.request)
	var dialog *mygo.Window
	finish := func(allowed bool) {
		pending.resolve(allowed && c.ctx.Err() == nil && !c.window.IsDestroyed())
		dialog.Close()
	}
	dialog = mygo.NewWindow(mygo.WindowOptions{
		Parent: c.window, Modal: true, Title: model.title, Width: 440, Height: 300,
		UseContentSize: true, Frameless: true, CornerRadius: 20, SkipTaskbar: true,
		DisableResize: true, DisableMinimize: true, DisableMaximize: true, DisableFullScreen: true,
		BackgroundColor: "light-dark(#ffffff,#252733)",
		Content:         ui.View(func(ctx *ui.Context) { permissionView(ctx, model, finish) }),
	})
	c.permissionWindow = dialog
	dialog.OnClosed(func() {
		pending.resolve(false)
		c.permissionWindow = nil
		if len(c.permissionQueue) > 0 && c.permissionQueue[0] == pending {
			c.permissionQueue = c.permissionQueue[1:]
		}
		c.showPermissionPrompt()
	})
	c.loadPermissionFont()
}

func (c *client) closePermissionPrompts() {
	pending := c.permissionQueue
	c.permissionQueue = nil
	for _, request := range pending {
		request.resolve(false)
	}
}

type permissionModel struct {
	title, origin, description, allow string
	icon                              *ui.SVG
}

func permissionContent(req mygo.PermissionRequest) permissionModel {
	m := permissionModel{origin: req.Origin, allow: "允许", icon: permissionIcons[mygo.PermissionNotifications]}
	if len(req.Permissions) != 1 {
		m.title = "允许使用这些权限？"
		label, _ := permissionLabel(req.Origin, req)
		m.description = "营地请求" + label + "，用于你正在使用的功能。"
		return m
	}
	m.icon = permissionIcons[req.Permissions[0]]
	switch req.Permissions[0] {
	case mygo.PermissionNotifications:
		m.title, m.allow = "开启桌面通知？", "允许通知"
		m.description = "及时收到回复、提及和聊天消息。"
	case mygo.PermissionCamera:
		m.title = "允许使用摄像头？"
		m.description = "使用摄像头拍摄照片或视频。"
	case mygo.PermissionMicrophone:
		m.title = "允许使用麦克风？"
		m.description = "使用麦克风录制声音。"
	case mygo.PermissionGeolocation:
		m.title = "允许获取当前位置？"
		m.description = "让营地获取你当前所在的位置。"
	}
	return m
}

func permissionTheme(dark bool) *ui.Theme {
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
		t.Background, t.Text, t.TextMuted = ui.Hex("#252733"), ui.Hex("#e5e7f0"), ui.Hex("#a2abc7")
		t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#363b4d"), ui.Hex("#41475c"), ui.Hex("#303548")
	} else {
		t.Background, t.Text, t.TextMuted = ui.Hex("#ffffff"), ui.Hex("#4b578c"), ui.Hex("#8991b3")
		t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#edf0f7"), ui.Hex("#e2e7f2"), ui.Hex("#d8e0ef")
	}
	t.Accent, t.AccentHover, t.AccentPressed = ui.Hex("#00a9b5"), ui.Hex("#08bac5"), ui.Hex("#0096a1")
	t.AccentText, t.InactiveAccent, t.Focus = ui.Hex("#ffffff"), t.Accent, t.Accent.Alpha(.65)
	t.Border, t.Radius, t.FontSize = ui.Transparent, 22, 14
	t.Font = "MiSans, Microsoft YaHei, sans-serif"
	return t
}

func permissionView(c *ui.Context, model permissionModel, finish func(bool)) {
	t := permissionTheme(c.Theme().Dark)
	c.OnWindowShortcut(0, ui.KeyEscape, func() { finish(false) })
	ui.Themed(c, t).Fill().Padding(windowCaptionHeight+4, 28, 28, 28).Gap(20).Radius(20).Background(t.Background).
		Role(ui.RoleDialog).Label(model.title).Children(func() {
		ui.Row(c).FillWidth().Gap(16).Height(56).Shrink(0).Children(func() {
			ui.Box(c).Size(56, 56).Radius(18).Background(t.Accent.Alpha(.12)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Icon(c, model.icon).Size(28, 28).TextColor(t.Accent)
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(6).Padding(0, 8, 0, 0).Children(func() {
				ui.Text(c, model.title).FontSize(21)
				ui.Text(c, model.origin).FontSize(12).TextColor(t.TextMuted).SingleLine()
			})
		})
		ui.Text(c, model.description).FillWidth().FontSize(14).TextColor(t.Text)
		ui.Spacer(c)
		ui.Row(c).FillWidth().Gap(12).Shrink(0).Children(func() {
			ui.Button(c.Key("deny"), "暂不允许").Grow(1).Height(44).AutoFocus().OnClick(func() { finish(false) })
			ui.PrimaryButton(c.Key("allow"), model.allow).Grow(1).Height(44).OnClick(func() { finish(true) })
		})
		ui.Box(c).Absolute().Left(0).Right(42).Top(0).Height(windowCaptionHeight).DragWindow()
		nativeCaptionControl(c, t, "close", "关闭权限请求", permissionCloseIcon, func() { finish(false) }).Absolute().Right(0).Top(0)
	})
}

func permissionSVG(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + strings.TrimSpace(shapes) + `</svg>`))
}

var permissionIcons = map[mygo.Permission]*ui.SVG{
	mygo.PermissionNotifications: permissionSVG(`<path d="M9 18h6M10 21h4M5 17h14l-2-3V9a5 5 0 0 0-10 0v5z"/><path d="M12 3V2"/>`),
	mygo.PermissionCamera:        permissionSVG(`<path d="M8 5l2-2h4l2 2h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2z"/><circle cx="12" cy="13" r="4"/>`),
	mygo.PermissionMicrophone:    permissionSVG(`<rect x="9" y="2" width="6" height="12" rx="3"/><path d="M5 10v2a7 7 0 0 0 14 0v-2M12 19v3M8 22h8"/>`),
	mygo.PermissionGeolocation:   permissionSVG(`<path d="M19 10c0 5-7 12-7 12S5 15 5 10a7 7 0 0 1 14 0z"/><circle cx="12" cy="10" r="2.5"/>`),
}

var permissionCloseIcon = captionSVG(`<path d="m.5.5 9 9m-9 0 9-9"/>`)
