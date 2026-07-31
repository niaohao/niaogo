//go:build !windows

package conslog

import "os"

func enableVT() bool {
	// 非 Windows：TTY 或显式 TERM 时开色
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return true
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
