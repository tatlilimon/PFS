package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeWrappers(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, f := range []string{"pfs.bash", "pfs.zsh", "pfs.fish"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("# wrapper\n"), 0o644))
	}
}

func TestFindWrapperDir(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "PFS")
	writeWrappers(t, filepath.Join(repo, "shell"))
	prefix := filepath.Join(root, "usr")
	writeWrappers(t, filepath.Join(prefix, "share", "pfs", "shell"))

	// Binary built in the repository root (make build).
	assert.Equal(t, filepath.Join(repo, "shell"), findWrapperDir(filepath.Join(repo, "pfs"), nil))
	// Binary in <repo>/bin.
	assert.Equal(t, filepath.Join(repo, "shell"), findWrapperDir(filepath.Join(repo, "bin", "pfs"), nil))
	// Installed with make install: <prefix>/bin and <prefix>/share/pfs/shell.
	assert.Equal(t, filepath.Join(prefix, "share", "pfs", "shell"), findWrapperDir(filepath.Join(prefix, "bin", "pfs"), nil))
	// Fallback list.
	assert.Equal(t, filepath.Join(repo, "shell"), findWrapperDir(filepath.Join(root, "elsewhere", "pfs"), []string{filepath.Join(repo, "shell")}))
	assert.Equal(t, "", findWrapperDir(filepath.Join(root, "elsewhere", "pfs"), nil))
}

func TestSetup_PrintsSourceLineForShell(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/fish")
	stdout, _, err := runRoot(t, "setup")
	require.NoError(t, err)
	assert.Contains(t, stdout, "config.fish")
	line := ""
	for _, l := range strings.Split(stdout, "\n") {
		if strings.Contains(l, "source ") && strings.Contains(l, "pfs.fish") {
			line = l
		}
	}
	require.NotEmpty(t, line, stdout)
	assert.NotContains(t, line, "shell/shell", "the wrapper path must not repeat the directory")
}
