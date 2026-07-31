package conslog

import (
	"fmt"
	"os"
	"sync"
)

// ANSI：黄字用于未实现命令 / 缺资源，方便扫日志。
const (
	ansiYellow = "\033[33m"
	ansiReset  = "\033[0m"
)

var (
	once    sync.Once
	colored bool
)

// Enable 打开控制台彩色（Windows 需 VT 模式；失败则退回无色）。
func Enable() {
	once.Do(func() {
		colored = enableVT()
		if os.Getenv("NO_COLOR") != "" {
			colored = false
		}
	})
}

// Yellow 整行/片段包黄字。
func Yellow(s string) string {
	Enable()
	if !colored {
		return s
	}
	return ansiYellow + s + ansiReset
}

// Yellowf 格式化后黄字。
func Yellowf(format string, args ...any) string {
	return Yellow(fmt.Sprintf(format, args...))
}
