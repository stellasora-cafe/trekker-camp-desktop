// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package webclient

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

type guestCallbacks struct {
	OpenTool     func(string)
	CheckUpdates func()
	AboutCamp    func()
	LoadTool     func(context.Context, string) error
}

// guestPanel keeps native controls on the UI thread and loads cached site
// metadata immediately. Registration and login use separate WebView windows.
type guestPanel struct {
	ctx               context.Context
	cancel            context.CancelFunc
	origin, directory string
	resourceDir       string
	callbacks         guestCallbacks
	window            *mygo.Window
	name, description string
	loadError         string
	loadingPath       string
}

func newGuestPanel(ctx context.Context, origin, directory string, callbacks guestCallbacks) *guestPanel {
	ctx, cancel := context.WithCancel(ctx)
	p := &guestPanel{ctx: ctx, cancel: cancel, origin: origin, directory: directory, callbacks: callbacks,
		name: "旅人营地", description: "基于 Misskey 的星塔旅人非官方社区。爱与希望，就在路上。"}
	var cached struct{ Origin, Name, Description string }
	if data, err := os.ReadFile(filepath.Join(directory, "guest-meta.json")); err == nil && json.Unmarshal(data, &cached) == nil && cached.Origin == origin {
		p.applyMeta(cached.Name, cached.Description)
	}
	return p
}

func (p *guestPanel) Attach(window *mygo.Window) {
	mygo.RunOnMain(func() { p.window = window })
}

func (p *guestPanel) Close() {
	p.cancel()
	p.window = nil
}

func (p *guestPanel) update(fn func()) {
	mygo.RunOnMain(func() {
		if p.ctx.Err() != nil || p.window == nil || p.window.IsDestroyed() {
			return
		}
		fn()
		if p.window != nil && !p.window.IsDestroyed() {
			p.window.Invalidate()
		}
	})
}

var guestMarkup = regexp.MustCompile(`<[^>]*>`)

func (p *guestPanel) applyMeta(name, description string) {
	if strings.TrimSpace(name) != "" {
		p.name = name
	}
	if strings.TrimSpace(description) != "" {
		p.description = guestMarkup.ReplaceAllString(description, "")
	}
}

func (p *guestPanel) tool(path string) {
	if path == "/about-camp" && p.callbacks.AboutCamp != nil {
		p.callbacks.AboutCamp()
		return
	}
	if p.loadingPath != "" {
		return
	}
	if p.callbacks.LoadTool != nil {
		p.loadingPath, p.loadError = path, ""
		if p.window != nil {
			p.window.Invalidate()
		}
		go func() {
			err := p.callbacks.LoadTool(p.ctx, path)
			p.update(func() {
				p.loadingPath = ""
				if err != nil {
					p.loadError = err.Error()
				}
			})
		}()
		return
	}
	if p.callbacks.OpenTool != nil {
		p.callbacks.OpenTool(path)
	}
}

var guestTools = []struct {
	label, path string
	icon        *mygo.MenuIcon
}{
	{"服务器信息", "/about", guestMenuIcon(`<rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/><path d="M7 7.5h.01M7 16.5h.01"/>`)},
	{"公告", "/announcements", guestMenuIcon(`<path d="M3 10h4l12-5v14L7 14H3zM7 14l2 6M22 10v4"/>`)},
	{"搜索", "/search", guestMenuIcon(`<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 5 5"/>`)},
	{"查询用户", "/lookup", guestMenuIcon(`<circle cx="12" cy="8" r="3.5"/><path d="M5 20v-2a4 4 0 0 1 4-4h6a4 4 0 0 1 4 4v2"/>`)},
	{"关于营地", "/about-camp", guestMenuIcon(`<circle cx="12" cy="12" r="9"/><path d="M12 11v5m0-8v.01"/>`)},
}

var guestUpdateIcon = guestMenuIcon(`<path d="M20 9a8 8 0 0 0-13.4-3L4 9m0-5v5h5m-5 6a8 8 0 0 0 13.4 3l2.6-3m0 5v-5h-5"/>`)

func guestMenuIcon(path string) *mygo.MenuIcon {
	icon, err := mygo.NewMenuIconSVG(permissionSVG(path), true)
	if err != nil {
		panic(err)
	}
	return icon
}

func (p *guestPanel) View(c *ui.Context) {
	t := permissionTheme(false)
	background := ui.Hex("#eef2f8")
	c.SetMenuPresentation(ui.PopoverMenus)
	c.Root().Background(background)
	ui.Themed(c, t).Fill().Background(background).Draw(func(painter *ui.Painter, bounds ui.Rect) {
		painter.FillRadialGradient(bounds, ui.RadialGradient{
			Center: ui.Point{X: .82, Y: .08},
			Stops:  []ui.GradientStop{{At: 0, Color: ui.Hex("#aadfe0")}, {At: 1, Color: ui.Transparent}},
		}, 0)
		painter.FillRadialGradient(bounds, ui.RadialGradient{
			Center: ui.Point{X: .08, Y: 1},
			Stops:  []ui.GradientStop{{At: 0, Color: ui.Hex("#f6eac3")}, {At: 1, Color: ui.Transparent}},
		}, 0)
	}).Children(func() {
		ui.Column(c).Fill().Clip().Children(func() {
			p.titlebar(c, t)
			ui.Scroll(c.Key("guest-scroll")).FillWidth().Grow(1).MinHeight(0).Scrollbars(false).Children(func() {
				ui.Column(c).FillWidth().MaxWidth(460).AlignSelf(ui.Center).Padding(4, 20, 22, 20).Children(func() {
					tint := t.Background.Alpha(.70)
					ui.Column(c).FillWidth().Padding(20, 24, 18, 24).Gap(8).Radius(22).
						Material(guestGlass{tint: tint}).Border(1, t.Background.Alpha(.8)).
						Shadow(0, 8, 26, 0, ui.RGBA(60, 72, 106, .12)).Children(func() {
						ui.Image(c, guestBrand).Size(60, 60).Radius(13).Clip().AlignSelf(ui.Center).Shrink(0)
						ui.Text(c, p.name).FillWidth().TextAlign(ui.Center).FontSize(22).Bold().SingleLine().Shrink(0)
						p.welcomeView(c)
					})
				})
			})
			p.toolsMenu(c, t)
		})
	})
}

type guestGlass struct{ tint ui.Color }

func (g guestGlass) PaintMaterial(p *ui.Painter, bounds ui.Rect, corners [4]float32) {
	glass.Blur{Radius: 14}.PaintMaterial(p, bounds, corners)
	p.FillRadii(bounds, corners, g.tint)
}

func (p *guestPanel) toolsMenu(c *ui.Context, t *ui.Theme) {
	trigger := ui.ButtonBase(c.Key("menu")).Absolute().Right(30).Top(64).Size(34, 34).Radius(10).ZIndex(2).Label("更多选项").
		Background(t.Background.Alpha(.65)).AlignItems(ui.Center).Justify(ui.Center)
	if trigger.Hovered() {
		trigger.Background(t.SurfaceHover)
	}
	trigger.Children(func() {
		ui.Icon(c, guestMoreIcon).Size(18, 18)
	}).PopoverMenuButton(func(m *ui.Menu) {
		for _, item := range guestTools {
			if item.path == "/about-camp" {
				m.Separator()
			}
			if m.Item(item.label).Icon(item.icon).Chosen() {
				p.tool(item.path)
			}
		}
		if m.Item("检查更新").Icon(guestUpdateIcon).Chosen() && p.callbacks.CheckUpdates != nil {
			p.callbacks.CheckUpdates()
		}
	}, ui.PopoverMenuButtonOptions{
		At: ui.AnchorBottomEnd, Self: ui.AnchorTopEnd,
		Panel: func(panel ui.Element) {
			panel.Radius(14).MinWidth(170).Padding(6).
				Background(ui.Transparent).Material(guestGlass{tint: t.Background.Alpha(.91)}).
				Border(1, t.Background.Alpha(.88)).NoShadow().Shadow(0, 8, 26, 0, ui.RGBA(46, 59, 94, .22))
		},
	})
}

func (p *guestPanel) titlebar(c *ui.Context, t *ui.Theme) {
	ui.Row(c).FillWidth().Height(windowCaptionHeight).Shrink(0).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Grow(1).Height(windowCaptionHeight).DragWindow()
		nativeCaptionControl(c, t, "minimize", "最小化", guestMinimizeIcon, func() {
			if p.window != nil {
				p.window.Minimize()
			}
		})
		nativeCaptionControl(c, t, "maximize", "最大化", guestMaximizeIcon, nil).Disabled(true)
		nativeCaptionControl(c, t, "close", "关闭窗口", campCloseIcon, func() {
			if p.window != nil {
				p.window.Close()
			}
		})
	})
}

func nativeCaptionControl(c *ui.Context, t *ui.Theme, key, label string, icon *ui.SVG, action func()) ui.Element {
	button := ui.ButtonBase(c.Key(key)).Size(windowCaptionWidth, windowCaptionHeight).Label(label).OnClick(action)
	if button.Hovered() {
		if key == "close" {
			button.Background(ui.Hex("#e81123")).TextColor(ui.Hex("#ffffff"))
		} else {
			button.Background(t.SurfaceHover)
		}
	}
	button.AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Icon(c, icon).Size(10, 10) })
	return button
}

func (p *guestPanel) welcomeView(c *ui.Context) {
	if p.loadingPath != "" {
		ui.Row(c).FillWidth().Justify(ui.Center).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Spinner(c).Size(18, 18)
			ui.Text(c, "正在加载…").FontSize(14)
		})
	} else if p.loadError != "" {
		ui.Text(c, p.loadError).FillWidth().TextAlign(ui.Center).FontSize(14).TextColor(ui.Hex("#e5484d")).Shrink(0)
	} else {
		ui.Text(c, p.description).FillWidth().TextAlign(ui.Center).FontSize(14).MaxLines(3).Shrink(0)
	}
	ui.Column(c).FillWidth().Gap(10).Padding(6, 0, 0, 0).Shrink(0).Children(func() {
		ui.PrimaryButton(c.Key("signup"), "注册").Label("注册").FillWidth().Height(42).Radius(21).Shrink(0).Disabled(p.loadingPath != "").OnClick(func() { p.tool("/desktop-auth?mode=signup") })
		ui.Button(c.Key("signin"), "登录").Label("登录").FillWidth().Height(42).Radius(21).Shrink(0).Disabled(p.loadingPath != "").OnClick(func() { p.tool("/desktop-auth?mode=signin") })
		ui.Button(c.Key("toolbox"), "不登录使用星塔工具箱（待开放）").Label("不登录使用星塔工具箱（待开放）").FillWidth().Height(42).Radius(21).Shrink(0).Disabled(true)
	})
}

func captionSVG(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1">` + shapes + `</svg>`))
}

var guestMinimizeIcon = campMinimizeIcon
var guestMaximizeIcon = campMaximizeIcon
var guestMoreIcon = permissionSVG(`<circle cx="4" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="20" cy="12" r="1"/>`)

// The public camp mark matches assets/trekker-camp/branding/icon-v1.svg.
var guestBrand = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1254 1254" width="1254" height="1254"><rect width="1254" height="1254" fill="#103923"/><g fill="#fff1ce"><path d="M706 391H758C783 503 834 593 905 666C762 631 568 784 272 810C487 694 636 535 706 391Z"/><path d="M356 830C581 801 758 670 889 685C902 812 947 918 1006 987Q1010 995 1000 996H807L785 831L685 743L585 832L554 996H262C301 932 334 879 356 830Z"/></g><g fill="#f4cb69"><path d="M733 213C742 246 754 265 786 280C760 290 747 306 738 331H727C717 305 703 291 680 280C711 267 724 247 733 213Z"/><path d="M700 344H767Q773 344 776 351L786 370Q789 379 779 379H688Q677 379 681 370L691 351Q694 344 700 344Z"/></g><path fill="#9bd0ef" d="M685 798C695 838 712 861 751 878C716 895 697 920 685 957C673 918 652 892 618 878C655 862 674 837 685 798Z"/></svg>`))
