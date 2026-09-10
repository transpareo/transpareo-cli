package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/transpareo/transpareo-cli/pkg/transpareo/signer"
)

func TestSignerKeygenWritesKeysAndPrintsPublicHalves(t *testing.T) {
	h := newHarness(t)
	h.terminal = true
	dir := filepath.Join(t.TempDir(), "keys")
	out, errOut, code := h.run("signer", "keygen", "--dir", dir)
	if code != 0 {
		t.Fatalf("keygen: %d %s%s", code, out, errOut)
	}
	if !strings.Contains(out, "P-256 public key") ||
		strings.Count(out, "-----BEGIN PUBLIC KEY-----") != 2 {
		t.Errorf("out = %s", out)
	}
	for _, name := range []string{"p256.pem", "ed25519.pem"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		// Windows has no owner-only mode to check.
		if perm := info.Mode().Perm(); perm != 0o600 &&
			runtime.GOOS != "windows" {
			t.Errorf("%s mode = %o", name, perm)
		}
	}
	if _, err := signer.LoadP256(filepath.Join(dir, "p256.pem")); err != nil {
		t.Error(err)
	}
	if _, err := signer.LoadEd25519(filepath.Join(dir,
		"ed25519.pem")); err != nil {
		t.Error(err)
	}

	// A second run keeps the keys unless forced.
	out, errOut, code = h.run("signer", "keygen", "--dir", dir)
	if code != 2 || !strings.Contains(out+errOut, "--force") {
		t.Errorf("without --force: %d %s%s", code, out, errOut)
	}
	h.terminal = false
	before, _ := os.ReadFile(filepath.Join(dir, "p256.pem"))
	out, _, code = h.run("signer", "keygen", "--dir", dir, "--force", "--json")
	if code != 0 {
		t.Fatalf("with --force: %d %s", code, out)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "p256.pem"))
	if string(before) == string(after) {
		t.Error("--force kept the old key")
	}
	var report map[string]map[string]string
	if err := json.Unmarshal([]byte(out), &report); err != nil ||
		report["p256"]["path"] != filepath.Join(dir, "p256.pem") ||
		!strings.HasPrefix(report["ed25519"]["publicKey"], "-----BEGIN") {
		t.Errorf("json = %s (%v)", out, err)
	}
}

// syncBuffer is a bytes.Buffer safe to read while the server
// goroutine writes its log to it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestSignerServeSignsAndStops runs the endpoint on a free port
// with the platform signature turned off, signs a request
// through it, and stops it through the context.
func TestSignerServeSignsAndStops(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	if _, _, code := h.run("signer", "keygen", "--dir", dir); code != 0 {
		t.Fatal("keygen failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stderr := &syncBuffer{}
	app := &App{Stdin: strings.NewReader(""), Stdout: io.Discard,
		Stderr: stderr, Getenv: func(string) string { return "" },
		ConfigDir: h.configDir, WorkDir: t.TempDir(), Stores: h.stores}
	done := make(chan int, 1)
	go func() {
		done <- Main(ctx, app, []string{"signer", "serve", "--allow-unsigned",
			"--listen", "127.0.0.1:0", "--p256-key",
			filepath.Join(dir, "p256.pem"), "--ed25519-key",
			filepath.Join(dir, "ed25519.pem")})
	}()
	url := waitForURL(t, stderr, done)
	body := `{"snapshots":[{"kind":"test","body":"{}","proofs":[{"a":1}]}]}`
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(data), `"z`) {
		t.Errorf("serve answered %d %s", resp.StatusCode, data)
	}
	if !strings.Contains(stderr.String(), "kind=documents") {
		t.Errorf("log = %s", stderr.String())
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("serve exited %d: %s", code, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop")
	}
}

func TestSignerServeNeedsKeysAndAPlatformKey(t *testing.T) {
	h := newHarness(t)
	out, errOut, code := h.run("signer", "serve", "--allow-unsigned",
		"--p256-key", filepath.Join(t.TempDir(), "none.pem"))
	if code != 1 || !strings.Contains(out, "signer keygen") {
		t.Errorf("missing key: %d %s%s", code, out, errOut)
	}
	out, errOut, code = h.run("signer", "serve")
	if code != 2 || !strings.Contains(out, "--platform-key") {
		t.Errorf("no platform key: %d %s%s", code, out, errOut)
	}
	out, errOut, code = h.run("signer", "serve", "--platform-key", "x.pem",
		"--tls-cert", "c.pem")
	if code != 2 || !strings.Contains(out, "go together") {
		t.Errorf("half a TLS pair: %d %s%s", code, out, errOut)
	}
}

// waitForURL reads the "ready" line the server logs and answers
// the URL it names.
func waitForURL(t *testing.T, stderr *syncBuffer, done chan int) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case code := <-done:
			t.Fatalf("serve exited early with %d: %s", code, stderr.String())
		default:
		}
		for _, line := range strings.Split(stderr.String(), "\n") {
			if i := strings.Index(line, "url=http://"); i >= 0 {
				return strings.Fields(line[i+4:])[0]
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the server never reported its address")
	return ""
}

// TestSignerStaysOutOfTheSkill proves the assistant's skill does
// not name the operator commands.
func TestSignerStaysOutOfTheSkill(t *testing.T) {
	app := &App{Getenv: func(string) string { return "" }}
	skill, err := os.ReadFile("../../skills/transpareo/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := FillReference(string(skill), app.Root())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, "transpareo signer") {
		t.Error("the skill names the signer")
	}
	if !strings.Contains(rendered, "transpareo tasks wait") {
		t.Error("the skill lost an ordinary command")
	}
}
