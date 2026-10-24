//go:build desktop

package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestTrayExitConfirmationAcceptsNativeYes(t *testing.T) {
	for _, id := range []string{"", "local-instance"} {
		t.Run("instance="+id, func(t *testing.T) {
			approved, err := confirmLocalCoreExit(id, func(options wailsruntime.MessageDialogOptions) (string, error) {
				if !strings.Contains(options.Message, "不会停止远程设备上的核心") {
					t.Fatal("confirmation omitted local-only scope")
				}
				if id == "" {
					if !strings.Contains(options.Message, "本机核心未运行") {
						t.Fatal("confirmation invented a running core")
					}
				} else if !strings.Contains(options.Message, "实例："+id) || !strings.Contains(options.Message, "尝试恢复托管配置") {
					t.Fatal("confirmation omitted target or stop impact")
				}
				if options.DefaultButton != "No" || options.CancelButton != "No" || !slices.Equal(options.Buttons, []string{"Yes", "No"}) {
					t.Fatal("confirmation must request native negative defaults", options)
				}
				// Model the Windows/Linux Wails backend: custom button labels
				// are ignored, warnings return OK, questions return Yes/No.
				if options.Type == wailsruntime.QuestionDialog {
					return "Yes", nil
				}
				return "OK", nil
			})
			if err != nil || !approved {
				t.Fatal("native approval was treated as cancellation", approved, err)
			}
		})
	}
}

func TestTrayExitConfirmationRejectsOtherResponsesAndErrors(t *testing.T) {
	dialogErr := errors.New("dialog unavailable")
	for _, response := range []string{"No", "Cancel", "", "OK", "Ok", "停止核心并退出", "Yes"} {
		for _, failure := range []error{nil, dialogErr} {
			t.Run(fmt.Sprintf("%s/error=%t", response, failure != nil), func(t *testing.T) {
				approved, err := confirmLocalCoreExit("local-instance", func(wailsruntime.MessageDialogOptions) (string, error) {
					return response, failure
				})
				if approved != (response == "Yes" && failure == nil) || err != failure {
					t.Fatal("unexpected approval or lost dialog error", approved, err)
				}
			})
		}
	}
}
