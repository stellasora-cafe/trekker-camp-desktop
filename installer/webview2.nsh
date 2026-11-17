; SPDX-FileCopyrightText: stellasora-cafe contributors
; SPDX-License-Identifier: Apache-2.0

!include "LogicLib.nsh"

!macro customInstall
  ExecWait '"$INSTDIR\TrekkerCamp.exe" --check-webview-runtime' $0
  ${If} $0 != 0
    IfSilent camp_webview_done
    MessageBox MB_YESNO|MB_ICONINFORMATION "尚未检测到可用的 Microsoft Edge WebView2 Runtime。旅人营地需要此组件才能运行。现在打开微软官方下载页面吗？安装完成后再启动旅人营地。" /SD IDNO IDNO camp_webview_done
    ExecShell "open" "https://developer.microsoft.com/microsoft-edge/webview2/#download"
  ${EndIf}
  camp_webview_done:
!macroend

!macro customUnInstall
  IfSilent camp_uninstall_keep_data
  MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 "是否同时清除旅人营地的本机登录状态、设置、草稿和缓存？此操作无法撤销。" /SD IDNO IDNO camp_uninstall_keep_data
  ClearErrors
  ExecWait '"$INSTDIR\TrekkerCamp.exe" --clear-user-data' $0
  IfErrors camp_uninstall_cleanup_failed
  IntCmp $0 0 camp_uninstall_keep_data
  camp_uninstall_cleanup_failed:
  MessageBox MB_OK|MB_ICONEXCLAMATION "部分本机数据未能清除。请退出其他旅人营地窗口，再检查 %APPDATA% 和 %LOCALAPPDATA% 下的旅人营地文件夹。"
  camp_uninstall_keep_data:
!macroend
