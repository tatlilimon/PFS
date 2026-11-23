package main

import (
	"os"
	"testing"
)

// Tests drive the non-interactive code paths. Never start the TUI from a test,
// even when the test binary runs in a terminal. HOME and XDG_CONFIG_HOME point
// to a scratch directory so no test can read, migrate or rename the real
// ~/.pfs.env or ~/.config/pfs/config.yaml.
func TestMain(m *testing.M) {
	isInteractive = func() bool { return false }
	os.Unsetenv("PFS_EVAL")
	home, err := os.MkdirTemp("", "pfs-cmd-test-home-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", home+"/.config")
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
