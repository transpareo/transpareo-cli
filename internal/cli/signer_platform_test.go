package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/transpareo/transpareo-cli/pkg/transpareo/signer"
)

// TestSignerServeFollowsARotation runs the endpoint against a
// host that rotates its request-signing key: a request signed
// with the pinned key passes, and after the host promotes a new
// key, a request signed with that one passes too, because the
// hand-over statement carries it in.
func TestSignerServeFollowsARotation(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	if _, _, code := h.run("signer", "keygen", "--dir", dir); code != 0 {
		t.Fatal("keygen failed")
	}
	_, pinned, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	currentPublic, current, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	// The platform host: the key it signs with now, and the
	// statement the retired key signed over the hand-over.
	served := pinned
	rotated := false
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "-rotation.json") {
			if !rotated {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			writeRotation(t, w, pinned, currentPublic)
			return
		}
		pem, err := signer.PublicPEM(served)
		if err != nil {
			t.Error(err)
		}
		w.Write(pem)
	}))
	defer platform.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stderr := &syncBuffer{}
	app := &App{Stdin: strings.NewReader(""), Stdout: io.Discard,
		Stderr: stderr, Getenv: func(string) string { return "" },
		ConfigDir: h.configDir, WorkDir: t.TempDir(), Stores: h.stores}
	done := make(chan int, 1)
	go func() {
		done <- Main(ctx, app, []string{"signer", "serve", "--dir", dir,
			"--listen", "127.0.0.1:0", "--host", "signer.example.com",
			"--platform-key", platform.URL + "/transpareo-signing-key.pem"})
	}()
	url := waitForURL(t, stderr, done)

	if status := signedPost(t, url, pinned); status != 200 {
		t.Errorf("the pinned key: %d %s", status, stderr.String())
	}

	// The host promotes the new key and publishes the statement.
	served, rotated = current, true
	if status := signedPost(t, url, current); status != 200 {
		t.Errorf("after the rotation: %d %s", status, stderr.String())
	}

	// A key nobody handed over to stays out.
	_, stranger, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	if status := signedPost(t, url, stranger); status != 401 {
		t.Errorf("a key of its own: %d", status)
	}
	cancel()
	<-done
}

// writeRotation publishes the statement in which previous
// retires in favour of successor.
func writeRotation(t *testing.T, w http.ResponseWriter,
	previous ed25519.PrivateKey, successor ed25519.PublicKey) {
	t.Helper()
	previousFingerprint, err := signer.Fingerprint(
		previous.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := signer.Fingerprint(successor)
	if err != nil {
		t.Fatal(err)
	}
	rotatedAt := "2026-09-10T18:16:06Z"
	message := strings.Join([]string{"master-key-rotation",
		previousFingerprint, fingerprint, rotatedAt}, "\n")
	previousPEM, err := signer.PublicPEM(previous)
	if err != nil {
		t.Fatal(err)
	}
	json.NewEncoder(w).Encode(map[string]string{
		"previous_fingerprint": previousFingerprint,
		"previous_public_key":  string(previousPEM),
		"fingerprint":          fingerprint,
		"rotated_at":           rotatedAt,
		"signature": base64.StdEncoding.EncodeToString(
			ed25519.Sign(previous, []byte(message))),
	})
}

// signedPost sends a documents request signed the way the
// platform signs one, and answers the status.
func signedPost(t *testing.T, url string, key ed25519.PrivateKey) int {
	t.Helper()
	body := []byte(`{"snapshots":[{"kind":"test","body":"{}",` +
		`"proofs":[{"a":1}]}]}`)
	timestamp := time.Now().UTC().Format(time.RFC3339)
	nonce := fmt.Sprintf("%s-%x", timestamp,
		key.Public().(ed25519.PublicKey)[:4])
	fingerprint, err := signer.Fingerprint(key.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	canonical := signer.Canonical(http.MethodPost, "signer.example.com",
		"/sign", timestamp, nonce, body)
	req.Host = "signer.example.com"
	req.Header.Set(signer.HeaderTimestamp, timestamp)
	req.Header.Set(signer.HeaderNonce, nonce)
	req.Header.Set(signer.HeaderFingerprint, fingerprint)
	req.Header.Set(signer.HeaderSignature, base64.StdEncoding.EncodeToString(
		ed25519.Sign(key, canonical)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// TestSignerReadsTheRotationStatement checks where the endpoint
// looks for the hand-over statement and what it makes of the two
// answers a host gives: the statement, or nothing at all.
func TestSignerReadsTheRotationStatement(t *testing.T) {
	const key = "https://acme.transpareo.com/.well-known/" +
		"transpareo-signing-key.pem"
	const want = "https://acme.transpareo.com/.well-known/" +
		"transpareo-signing-key-rotation.json"
	if got := rotationURL(key); got != want {
		t.Errorf("rotationURL = %s", got)
	}

	statement := `{"previous_fingerprint":"56dc","previous_public_key":"pem",
		"fingerprint":"850d","rotated_at":"2026-09-10T18:16:06Z",
		"signature":"c2ln"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {
		if r.URL.Path != "/rotation.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, statement)
	}))
	defer server.Close()
	app := &App{Getenv: func(string) string { return "" }}
	rotation, err := app.fetchRotation(context.Background(),
		server.URL+"/rotation.json")
	if err != nil || rotation.Fingerprint != "850d" {
		t.Errorf("statement = %v (%v)", rotation, err)
	}
	_, err = app.fetchRotation(context.Background(), server.URL+"/none.json")
	if !errors.Is(err, signer.ErrNoRotation) {
		t.Errorf("a host that publishes none: %v", err)
	}
}
