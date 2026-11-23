package shell

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeOllama answers each generate request with the next prepared correction.
type fakeOllama struct {
	mu      sync.Mutex
	replies []string
	served  int
}

func (f *fakeOllama) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodHead {
		return
	}
	f.mu.Lock()
	reply := `{"diagnosis":"no more prepared replies","corrected_command":null,"confidence":"low"}`
	if f.served < len(f.replies) {
		reply = f.replies[f.served]
	}
	f.served++
	f.mu.Unlock()
	data, _ := json.Marshal(map[string]interface{}{"response": reply, "done": true})
	w.Write(append(data, '\n'))
}

func correction(command string) string {
	data, _ := json.Marshal(map[string]interface{}{
		"diagnosis":         "The command name is misspelled.",
		"corrected_command": command,
		"confidence":        "high",
	})
	return string(data)
}

// End to end: wrapper -> real pfs binary -> fake Ollama -> TUI in a terminal.
// Only the command confirmed with r runs; dismissing or backing out of a
// destructive command runs nothing.
func TestEndToEnd_RealBinaryInTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary and drives real shells")
	}
	forEachShell(t, func(t *testing.T, sh shellCase) {
		s := newSession(t)
		build := exec.Command("go", "build", "-o", filepath.Join(s.bin, "pfs"), "github.com/tatlilimon/PFS/cmd")
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build pfs: %v\n%s", err, out)
		}

		ollama := &fakeOllama{replies: []string{
			correction(`echo FIXED > "$FAKEPFS_DIR/fixed"`),
			correction(`echo NO > "$FAKEPFS_DIR/dismissed"`),
			correction(`git reset --hard HEAD`),
		}}
		server := httptest.NewServer(ollama)
		defer server.Close()
		cfgDir := filepath.Join(s.home, ".config", "pfs")
		if err := os.MkdirAll(cfgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		cfg := "ollama_base_url: " + server.URL + "\nollama_model: test-model\n"
		if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}

		transcript := s.runItems(sh.start(s, ""), [][]interface{}{
			{"lsx-not-a-command", "pfs", expect("[r]", "r")},
			{"lsx-not-a-command", "pfs", expect("[x]", "x")},
			{"lsx-not-a-command", "pfs", expect("Destructive", "r"), expect("Press y", "n"), expect("[x]", "x")},
		})

		if s.read("fixed") != "FIXED\n" {
			t.Errorf("the confirmed command did not run\n%s", transcript)
		}
		if s.exists("dismissed") {
			t.Errorf("a dismissed command ran\n%s", transcript)
		}
		if strings.Contains(transcript, "not a git repository") {
			t.Errorf("a destructive command ran after backing out\n%s", transcript)
		}
		ollama.mu.Lock()
		served := ollama.served
		ollama.mu.Unlock()
		if served != 3 {
			t.Errorf("want 3 requests to Ollama, got %d\n%s", served, transcript)
		}
	})
}

func expect(text, keys string) map[string]string {
	return map[string]string{"expect": text, "keys": keys}
}

// runItems is run with steps that may wait for output and send raw keys.
func (s *session) runItems(argv []string, steps [][]interface{}) string {
	s.t.Helper()
	wd, _ := os.Getwd()
	driver := filepath.Join(wd, "testdata", "drive.py")
	argvJSON, _ := json.Marshal(argv)
	stepsJSON, _ := json.Marshal(steps)
	cmd := exec.Command("python3", "-I", driver, string(argvJSON), string(stepsJSON), "20")
	cmd.Dir = s.home
	cmd.Env = s.env(wd)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		s.t.Fatalf("driver failed: %v\n%s\ntranscript:\n%s", err, stderr.String(), out)
	}
	return string(out)
}
