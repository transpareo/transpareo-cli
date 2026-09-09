package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

type fakeVerifier struct {
	err    error
	called bool
}

func (f *fakeVerifier) Verify(bundle, artifact []byte) error {
	f.called = true
	if string(bundle) != "BUNDLE" ||
		!strings.Contains(string(artifact), "transpareo_") {
		return errors.New("unexpected inputs")
	}
	return f.err
}

func tarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg})
	tw.Write(content)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func zipped(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create(name)
	w.Write(content)
	zw.Close()
	return buf.Bytes()
}

// release serves a fake GitHub API and download host for one
// version.
type release struct {
	server  *httptest.Server
	version string
	assets  map[string][]byte
}

func newRelease(t *testing.T, version string,
	assets map[string][]byte) *release {
	t.Helper()
	r := &release{version: version, assets: assets}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/transpareo/transpareo-cli/releases/",
		func(w http.ResponseWriter, req *http.Request) {
			wanted := strings.TrimPrefix(req.URL.Path,
				"/repos/transpareo/transpareo-cli/releases/")
			if wanted != "latest" && wanted != "tags/v"+version {
				http.NotFound(w, req)
				return
			}
			var list []string
			for name := range assets {
				list = append(list,
					fmt.Sprintf(`{"name": %q, "browser_download_url": %q}`,
						name,
						r.server.URL+"/transpareo/transpareo-cli/releases/download/v"+
							version+"/"+name))
			}
			fmt.Fprintf(w, `{"tag_name": "v%s", "assets": [%s]}`, version,
				strings.Join(list, ","))
		})
	mux.HandleFunc("/transpareo/transpareo-cli/releases/download/",
		func(w http.ResponseWriter, req *http.Request) {
			name := filepath.Base(req.URL.Path)
			data, ok := assets[name]
			if !ok {
				http.NotFound(w, req)
				return
			}
			w.Write(data)
		})
	r.server = httptest.NewServer(mux)
	t.Cleanup(r.server.Close)
	return r
}

func assetsFor(t *testing.T, version, goos, goarch string,
	binary []byte) map[string][]byte {
	t.Helper()
	name := archiveNameFor(version, goos, goarch)
	var archive []byte
	if goos == "windows" {
		archive = zipped(t, "transpareo.exe", binary)
	} else {
		archive = tarGz(t, "transpareo", binary)
	}
	sum := sha256.Sum256(archive)
	checksums := hex.EncodeToString(sum[:]) + "  " + name + "\n"
	checksumsName := "transpareo_" + version + "_checksums.txt"
	return map[string][]byte{
		name:                             archive,
		checksumsName:                    []byte(checksums),
		checksumsName + ".sigstore.json": []byte("BUNDLE"),
	}
}

func options(r *release, dir string, verifier Verifier) Options {
	return Options{
		Current: "1.0.0", HTTP: r.server.Client(), APIBase: r.server.URL,
		DownloadBase: r.server.URL, GOOS: "linux", GOARCH: "amd64",
		Executable: filepath.Join(dir, "transpareo"), Verifier: verifier,
	}
}

func TestRunInstallsAVerifiedRelease(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "transpareo"), []byte("old"), 0o755)
	r := newRelease(t, "1.1.0", assetsFor(t, "1.1.0", "linux", "amd64",
		[]byte("new binary")))
	verifier := &fakeVerifier{}
	var steps []string
	opts := options(r, dir, verifier)
	opts.Progress = func(s string) { steps = append(steps, s) }
	result, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Installed || !result.Verified || result.To != "1.1.0" ||
		result.From != "1.0.0" {
		t.Errorf("result = %+v", result)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "transpareo"))
	if string(data) != "new binary" {
		t.Errorf("binary = %q", data)
	}
	info, _ := os.Stat(filepath.Join(dir, "transpareo"))
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %o", info.Mode().Perm())
	}
	if !verifier.called || len(steps) < 3 {
		t.Errorf("verifier called %v, steps %v", verifier.called, steps)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir,
		".transpareo-upgrade-*")); len(leftovers) > 0 {
		t.Errorf("temporary files left: %v", leftovers)
	}
}

func TestRunSkipsTheCurrentVersion(t *testing.T) {
	dir := t.TempDir()
	r := newRelease(t, "1.0.0", assetsFor(t, "1.0.0", "linux", "amd64",
		[]byte("same")))
	result, err := Run(context.Background(), options(r, dir, &fakeVerifier{}))
	if err != nil || result.Installed || result.To != "1.0.0" {
		t.Errorf("result = %+v, err = %v", result, err)
	}
}

func TestRunRefusesAnUnverifiedRelease(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "transpareo"), []byte("old"), 0o755)
	r := newRelease(t, "1.1.0", assetsFor(t, "1.1.0", "linux", "amd64",
		[]byte("evil")))
	_, err := Run(context.Background(), options(r, dir,
		&fakeVerifier{err: errors.New("bad cert")}))
	if !transpareo.IsCode(err, CodeUnverified) {
		t.Fatalf("err = %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "transpareo"))
	if string(data) != "old" {
		t.Error("the binary must stay untouched")
	}
}

func TestRunRefusesAChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "transpareo"), []byte("old"), 0o755)
	assets := assetsFor(t, "1.1.0", "linux", "amd64", []byte("new"))
	assets["transpareo_1.1.0_linux_amd64.tar.gz"] = tarGz(t, "transpareo",
		[]byte("swapped"))
	r := newRelease(t, "1.1.0", assets)
	_, err := Run(context.Background(), options(r, dir, &fakeVerifier{}))
	if !transpareo.IsCode(err, CodeChecksum) {
		t.Fatalf("err = %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "transpareo"))
	if string(data) != "old" {
		t.Error("the binary must stay untouched")
	}
}

func TestRunReportsMissingAssets(t *testing.T) {
	dir := t.TempDir()
	r := newRelease(t, "1.1.0", assetsFor(t, "1.1.0", "darwin", "arm64",
		[]byte("mac")))
	_, err := Run(context.Background(), options(r, dir, &fakeVerifier{}))
	if !transpareo.IsCode(err, CodeNoAsset) {
		t.Fatalf("err = %v", err)
	}
	opts := options(r, dir, &fakeVerifier{})
	opts.Version = "9.9.9"
	if _, err := Run(context.Background(), opts); !transpareo.IsCode(err,
		CodeNoRelease) {
		t.Errorf("unknown version: %v", err)
	}
}

func TestWindowsZipAndRename(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "transpareo.exe")
	os.WriteFile(target, []byte("old"), 0o755)
	r := newRelease(t, "1.1.0", assetsFor(t, "1.1.0", "windows", "amd64",
		[]byte("win")))
	opts := options(r, dir, &fakeVerifier{})
	opts.GOOS, opts.Executable = "windows", target
	result, err := Run(context.Background(), opts)
	if err != nil || !result.Installed {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	data, _ := os.ReadFile(target)
	old, _ := os.ReadFile(target + ".old")
	if string(data) != "win" || string(old) != "old" {
		t.Errorf("new %q old %q", data, old)
	}
}

func TestCheck(t *testing.T) {
	r := newRelease(t, "2.0.0", assetsFor(t, "2.0.0", "linux", "amd64",
		[]byte("x")))
	release, err := Check(context.Background(), Options{HTTP: r.server.Client(),
		APIBase: r.server.URL, DownloadBase: r.server.URL})
	if err != nil || release.Version != "2.0.0" || len(release.Assets) != 3 {
		t.Errorf("release = %+v, err = %v", release, err)
	}
}

func TestSigstoreIdentity(t *testing.T) {
	v := SigstoreVerifier{Repo: DefaultRepo}
	if !strings.Contains(v.IdentityRegexp(),
		`transpareo/transpareo-cli/\.github/workflows/release\.yaml@refs/tags/v`) {
		t.Errorf("identity = %s", v.IdentityRegexp())
	}
	if err := v.Verify([]byte("not a bundle"), []byte("x")); err == nil {
		t.Error("garbage must not verify")
	}
}
