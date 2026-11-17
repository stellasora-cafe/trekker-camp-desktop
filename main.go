// SPDX-FileCopyrightText: trekker-camp contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	_ "github.com/egoist/mygo/plugins/automation"
	"github.com/stellasora-cafe/trekker-camp-desktop/internal/webclient"
	_ "github.com/stellasora-cafe/trekker-camp-desktop/locales"
)

func main() {
	const productionURL = "https://camp.stellasora.cafe"
	development := os.Getenv("MYGO_ENV") == "development" || mygo.App.Version() == ""
	defaultURL := productionURL
	if development {
		defaultURL = "http://localhost:3000"
	}
	baseURL := flag.String("url", defaultURL, "营地地址；开发版默认连接 http://localhost:3000")
	dataDir := flag.String("data-dir", "", "独立的本地客户端数据目录和浏览器空间")
	safeMode := flag.Bool("safe-mode", false, "安全模式；禁用自定义主题、CSS 和插件，使用 --safe-mode=false 退出")
	checkRuntime := flag.Bool("check-webview-runtime", false, "检查 WebView2 运行时；供安装器使用")
	clearUserData := flag.Bool("clear-user-data", false, "清除默认本机数据；供卸载器使用")
	flag.Parse()
	if *checkRuntime {
		if mygo.CheckWebViewRuntime() != nil {
			os.Exit(2)
		}
		return
	}
	if *clearUserData {
		if err := webclient.ClearInstalledUserData(productionURL); err != nil {
			log.Fatal(err)
		}
		return
	}
	safeModeSpecified := false
	flag.Visit(func(value *flag.Flag) {
		if value.Name == "safe-mode" {
			safeModeSpecified = true
		}
	})
	if mygo.App.Version() == "" {
		mygo.App.SetName("旅人营地 Dev")
	} else {
		// Keep the existing user-data path and display name when packaging the
		// executable under an ASCII name.
		mygo.App.SetName("旅人营地")
	}
	mygo.App.SetLanguages("zh-CN")
	if *dataDir != "" {
		absolute, err := filepath.Abs(*dataDir)
		if err != nil {
			log.Fatal(err)
		}
		mygo.App.SetPath(mygo.PathUserData, absolute)
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	mygo.App.WhenReady(func() {
		if webclient.ShowRuntimeGuide() {
			mygo.App.OnWindowAllClosed(mygo.App.Quit)
			return
		}
		dir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			log.Fatal(err)
		}
		var safe *bool
		if safeModeSpecified {
			safe = safeMode
		}
		if err := webclient.Open(*baseURL, dir, safe); err != nil {
			mygo.Dialog.Error("旅人营地", err.Error())
			mygo.App.Quit()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
