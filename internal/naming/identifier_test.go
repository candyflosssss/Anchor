package naming

import (
	"strings"
	"testing"
)

func TestUserIdentifiers(t *testing.T) {
	for _, value := range []string{"key-1", "主力密钥", "备用_2.0", "中文模型", "𠮷野家", "Équipe", "１２号", strings.Repeat("鱼", 128)} {
		if !ValidID(value) {
			t.Errorf("valid identifier rejected: %q", value)
		}
	}
	for _, value := range []string{"", strings.Repeat("鱼", 129), "-中文", "_中文", ".中文", "中文 空格", "中文/路径", `中文\路径`, "中文:模型", "中文%2F路径", "中文?", "中文#", "中文\n", "中文\x00", "中文\u200b", "中文\u202e", "中文\u0301", "中文😀", string([]byte{0xff})} {
		if ValidID(value) {
			t.Errorf("unsafe identifier accepted: %q", value)
		}
	}
	if !ValidCode("request_failed") || ValidCode("中文错误") {
		t.Fatal("error code contract changed")
	}
}
