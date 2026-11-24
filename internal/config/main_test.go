package config

import (
	"os"
	"testing"
)

// Load migrates ~/.pfs.env when it exists. Point HOME at a scratch directory so
// the test suite can never rename a real user's file.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "pfs-config-test-home-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", home+"/.config")
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
