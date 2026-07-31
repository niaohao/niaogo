//go:build windows

package conslog

import (
	"os"
	"syscall"
	"unsafe"
)

func enableVT() bool {
	const enableVirtualTerminalProcessing = 0x0004
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")
	getStdHandle := kernel32.NewProc("GetStdHandle")

	enable := func(stdHandle int) bool {
		h, _, _ := getStdHandle.Call(uintptr(stdHandle))
		if h == 0 || h == ^uintptr(0) {
			return false
		}
		var mode uint32
		r, _, _ := getConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode)))
		if r == 0 {
			return false
		}
		mode |= enableVirtualTerminalProcessing
		r, _, _ = setConsoleMode.Call(h, uintptr(mode))
		return r != 0
	}
	// STD_OUTPUT_HANDLE=-11, STD_ERROR_HANDLE=-12
	okOut := enable(-11)
	okErr := enable(-12)
	if !okOut && !okErr {
		// 非控制台（管道/重定向）时仍尝试输出 ANSI，由终端决定是否解析
		return os.Getenv("TERM") != "" || os.Getenv("WT_SESSION") != "" || true
	}
	return true
}
