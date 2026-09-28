package cli

import (
	"fmt"
	"os/exec"
	"runtime"
)

// openBrowser is the default browser opener; tests replace it.
var openBrowser = startBrowser

// startBrowser opens url in the default browser without waiting for the
// browser to exit.
func startBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", cmd.Args[0], err)
	}
	go cmd.Wait()
	return nil
}
