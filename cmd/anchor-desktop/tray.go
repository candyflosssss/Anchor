//go:build desktop

package main

import (
	"context"
	"fmt"

	"github.com/candyflosssss/Anchor/internal/desktop"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func exitWithLocalCore(ctx context.Context, window *desktop.WindowLifecycle, session *desktop.Session) {
	// A tray action must not strand its confirmation behind a hidden window.
	if !window.Show() {
		return
	}
	var dialogErr error
	out := session.ExitWithLocalCore(func(instanceID string) bool {
		approved, err := confirmLocalCoreExit(instanceID, func(options wailsruntime.MessageDialogOptions) (string, error) {
			return wailsruntime.MessageDialog(ctx, options)
		})
		dialogErr = err
		return approved
	})
	if out.Problem == nil && dialogErr == nil {
		return
	}
	message := "无法确认本机核心已停止，Anchor 界面保持打开。请检查本机连接与实例状态后再操作；不会强制结束进程，也不会自动重试停服。"
	if dialogErr != nil {
		message = "无法显示退出确认，未发送停服请求，Anchor 界面保持打开。"
	} else if out.Problem.Code == "unsupported" {
		message = "本机核心不支持正常停服，Anchor 界面保持打开。请使用兼容版本的核心，或由原启动方式停止；不会强制结束进程。"
	} else if out.Problem.Code == "identity" {
		message = "本机实例身份已变化，未停止未经确认的实例，Anchor 界面保持打开。请重新检查本机连接。"
	}
	_, _ = wailsruntime.MessageDialog(ctx, wailsruntime.MessageDialogOptions{
		Type: wailsruntime.ErrorDialog, Title: "未退出 Anchor", Message: message, Buttons: []string{"确定"},
	})
}

func confirmLocalCoreExit(instanceID string, dialog func(wailsruntime.MessageDialogOptions) (string, error)) (bool, error) {
	message := "本机核心未运行。是否退出 Anchor（窗口和托盘）？\n不会停止远程设备上的核心。"
	if instanceID != "" {
		message = fmt.Sprintf("是否停止本机核心并退出 Anchor（窗口和托盘）？\n\n目标：本机\n实例：%s\n\n将停止本机代理服务，并按既有规则尝试恢复托管配置；依赖它的 Agent 请求会受影响。不会停止远程设备上的核心。", instanceID)
	}
	// Wails v2 on Windows/Linux ignores custom button labels. WarningDialog
	// only offers OK; QuestionDialog offers native Yes/No and returns these
	// English identifiers even when the OS displays translated button labels.
	button, err := dialog(wailsruntime.MessageDialogOptions{
		Type: wailsruntime.QuestionDialog, Title: "退出 Anchor", Message: message,
		Buttons: []string{"Yes", "No"}, DefaultButton: "No", CancelButton: "No",
	})
	return err == nil && button == "Yes", err
}
