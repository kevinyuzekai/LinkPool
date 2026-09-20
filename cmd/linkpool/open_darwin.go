//go:build darwin

package main

import (
	"fmt"
	"os/exec"
)

func openBrowser(url string) error {
	return exec.Command("open", url).Start()
}

// notifyUIReady shows a macOS notification pointing users to the Web UI
// (there is no native app window).
func notifyUIReady(url string) {
	msg := fmt.Sprintf("LinkPool 已启动，请打开 %s", url)
	script := fmt.Sprintf(`display notification %q with title "LinkPool"`, msg)
	_ = exec.Command("osascript", "-e", script).Start()
}
