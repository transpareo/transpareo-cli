// Package upgrade replaces the running binary with a release from
// GitHub after verifying the release's Sigstore signature and the
// archive's checksum. It is the one network call the tool makes
// besides the workspace host, and it happens only on an explicit
// `transpareo upgrade`.
package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/transpareo/transpareo-cli/pkg/transpareo"
)

// Error codes of the upgrade, in the envelope's code field.
const (
	CodeNoRelease  = "UPGRADE_NO_RELEASE"
	CodeNoAsset    = "UPGRADE_NO_ASSET"
	CodeDownload   = "UPGRADE_DOWNLOAD_FAILED"
	CodeUnverified = "UPGRADE_UNVERIFIED"
	CodeChecksum   = "UPGRADE_CHECKSUM_MISMATCH"
	CodeReplace    = "UPGRADE_REPLACE_FAILED"
)

// DefaultRepo is the GitHub repository releases come from.
const DefaultRepo = "transpareo/transpareo-cli"

// Verifier checks the Sigstore bundle of the checksum file.
type Verifier interface {
	Verify(bundle, artifact []byte) error
}

// Options drive Run and Check.
type Options struct {
	// Repo is "owner/name" on GitHub.
	Repo string

	// Version pins a release ("1.2.0" or "v1.2.0"); empty means
	// the latest.
	Version string

	// Current is the running version, so an upgrade to the same
	// one is skipped.
	Current string

	// HTTP is the client for GitHub; nil means the default.
	HTTP *http.Client

	// APIBase and DownloadBase replace the GitHub hosts in tests.
	APIBase      string
	DownloadBase string

	// GOOS and GOARCH select the archive; empty means the
	// running platform.
	GOOS, GOARCH string

	// Executable is the path to replace; empty means the running
	// binary.
	Executable string

	// Verifier checks the checksum file's bundle; nil means
	// Sigstore with the embedded trusted root.
	Verifier Verifier

	// Progress receives one line per step; nil discards.
	Progress func(string)
}

// Release names one release and its assets.
type Release struct {
	Version string            `json:"version"`
	Assets  map[string]string `json:"assets"`
}

// Result says what Run did.
type Result struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Verified  bool   `json:"verified"`
}

func (o *Options) defaults() {
	if o.Repo == "" {
		o.Repo = DefaultRepo
	}
	if o.HTTP == nil {
		o.HTTP = http.DefaultClient
	}
	if o.APIBase == "" {
		o.APIBase = "https://api.github.com"
	}
	if o.DownloadBase == "" {
		o.DownloadBase = "https://github.com"
	}
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.GOARCH == "" {
		o.GOARCH = runtime.GOARCH
	}
	if o.Verifier == nil {
		o.Verifier = SigstoreVerifier{Repo: o.Repo}
	}
	if o.Progress == nil {
		o.Progress = func(string) {}
	}
}

// Check finds the release without installing anything.
func Check(ctx context.Context, opts Options) (*Release, error) {
	opts.defaults()
	return fetchRelease(ctx, &opts)
}

func fetchRelease(ctx context.Context, opts *Options) (*Release, error) {
	url := opts.APIBase + "/repos/" + opts.Repo + "/releases/latest"
	if opts.Version != "" {
		url = opts.APIBase + "/repos/" + opts.Repo + "/releases/tags/v" +
			strings.TrimPrefix(opts.Version, "v")
	}
	data, err := fetch(ctx, opts.HTTP, url, "application/vnd.github+json")
	if err != nil {
		return nil, &transpareo.Error{Code: CodeNoRelease,
			Message:   "reading the release from GitHub: " + err.Error(),
			Retryable: true}
	}
	var doc struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(data, &doc); err != nil || doc.TagName == "" {
		return nil, &transpareo.Error{Code: CodeNoRelease,
			Message: "the release answer from GitHub is not what the client " +
				"expected"}
	}
	release := &Release{Version: strings.TrimPrefix(doc.TagName, "v"),
		Assets: map[string]string{}}
	for _, asset := range doc.Assets {
		release.Assets[asset.Name] = asset.URL
	}
	return release, nil
}

// Run downloads, verifies and installs the release.
func Run(ctx context.Context, opts Options) (*Result, error) {
	opts.defaults()
	release, err := fetchRelease(ctx, &opts)
	if err != nil {
		return nil, err
	}
	result := &Result{From: opts.Current, To: release.Version}
	if opts.Version == "" &&
		release.Version == strings.TrimPrefix(opts.Current, "v") {
		opts.Progress("transpareo " + release.Version + " is current")
		return result, nil
	}
	checksumsName := fmt.Sprintf("transpareo_%s_checksums.txt", release.Version)
	checksums, err := opts.asset(ctx, release, checksumsName)
	if err != nil {
		return nil, err
	}
	bundle, err := opts.asset(ctx, release, checksumsName+".sigstore.json")
	if err != nil {
		return nil, err
	}
	opts.Progress("verifying the Sigstore signature of " + checksumsName)
	if err := opts.Verifier.Verify(bundle, checksums); err != nil {
		return nil, &transpareo.Error{Code: CodeUnverified,
			Message: "the signature of the checksum file does not verify: " +
				err.Error(),
			Hint: "The release may be tampered with or the signing identity " +
				"changed; do not install it."}
	}
	result.Verified = true
	archiveName := archiveNameFor(release.Version, opts.GOOS, opts.GOARCH)
	archive, err := opts.asset(ctx, release, archiveName)
	if err != nil {
		return nil, err
	}
	if err := verifyChecksum(checksums, archiveName, archive); err != nil {
		return nil, err
	}
	binary, err := extract(archive, archiveName, opts.GOOS)
	if err != nil {
		return nil, err
	}
	path, err := opts.replace(binary)
	if err != nil {
		return nil, err
	}
	result.Installed = true
	result.Path = path
	opts.Progress("installed transpareo " + release.Version + " at " + path)
	return result, nil
}

func archiveNameFor(version, goos, goarch string) string {
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("transpareo_%s_%s_%s.%s", version, goos, goarch, ext)
}

func (o *Options) asset(ctx context.Context, release *Release,
	name string) ([]byte, error) {
	url, ok := release.Assets[name]
	if !ok {
		return nil, &transpareo.Error{Code: CodeNoAsset,
			Message: fmt.Sprintf("release %s has no asset %s", release.Version,
				name)}
	}
	if o.DownloadBase != "https://github.com" {
		if parsed, err := neturl.Parse(url); err == nil {
			url = o.DownloadBase + parsed.RequestURI()
		}
	}
	o.Progress("downloading " + name)
	data, err := fetch(ctx, o.HTTP, url, "application/octet-stream")
	if err != nil {
		return nil, &transpareo.Error{Code: CodeDownload,
			Message:   "downloading " + name + ": " + err.Error(),
			Retryable: true}
	}
	return data, nil
}

func fetch(ctx context.Context, client *http.Client, url,
	accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "transpareo-cli")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20))
}

// verifyChecksum finds the archive's line in the checksum file
// and compares the SHA-256.
func verifyChecksum(checksums []byte, name string, archive []byte) error {
	sum := sha256.Sum256(archive)
	actual := hex.EncodeToString(sum[:])
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			if fields[0] == actual {
				return nil
			}
			return &transpareo.Error{Code: CodeChecksum,
				Message: fmt.Sprintf("%s has checksum %s, the release says %s",
					name, actual, fields[0])}
		}
	}
	return &transpareo.Error{Code: CodeChecksum,
		Message: "the checksum file has no entry for " + name}
}

// extract returns the binary inside the archive.
func extract(archive []byte, name, goos string) ([]byte, error) {
	binaryName := "transpareo"
	if goos == "windows" {
		binaryName += ".exe"
	}
	if strings.HasSuffix(name, ".zip") {
		reader, err := zip.NewReader(bytes.NewReader(archive),
			int64(len(archive)))
		if err != nil {
			return nil, &transpareo.Error{Code: CodeDownload,
				Message: err.Error()}
		}
		for _, file := range reader.File {
			if filepath.Base(file.Name) != binaryName {
				continue
			}
			rc, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	} else {
		gz, err := gzip.NewReader(bytes.NewReader(archive))
		if err != nil {
			return nil, &transpareo.Error{Code: CodeDownload,
				Message: err.Error()}
		}
		tr := tar.NewReader(gz)
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, &transpareo.Error{Code: CodeDownload,
					Message: err.Error()}
			}
			if filepath.Base(header.Name) == binaryName &&
				header.Typeflag == tar.TypeReg {
				return io.ReadAll(tr)
			}
		}
	}
	return nil, &transpareo.Error{Code: CodeNoAsset,
		Message: name + " holds no " + binaryName}
}

// replace writes the binary beside the running one and renames it
// into place, so a failure leaves the old binary untouched.
func (o *Options) replace(binary []byte) (string, error) {
	target := o.Executable
	if target == "" {
		exe, err := os.Executable()
		if err != nil {
			return "", &transpareo.Error{Code: CodeReplace,
				Message: err.Error()}
		}
		target, err = filepath.EvalSymlinks(exe)
		if err != nil {
			return "", &transpareo.Error{Code: CodeReplace,
				Message: err.Error()}
		}
	}
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, ".transpareo-upgrade-*")
	if err != nil {
		return "", &transpareo.Error{Code: CodeReplace,
			Message: "writing next to " + target + ": " + err.Error(),
			Hint: "Check that the directory is writable, or install with " +
				"the package manager that put it there."}
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(binary); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", &transpareo.Error{Code: CodeReplace, Message: err.Error()}
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", &transpareo.Error{Code: CodeReplace, Message: err.Error()}
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		os.Remove(tmpPath)
		return "", &transpareo.Error{Code: CodeReplace, Message: err.Error()}
	}
	if o.GOOS == "windows" {
		old := target + ".old"
		os.Remove(old)
		if err := os.Rename(target, old); err != nil {
			os.Remove(tmpPath)
			return "", &transpareo.Error{Code: CodeReplace,
				Message: err.Error()}
		}
	}
	if err := os.Rename(tmpPath, target); err != nil {
		os.Remove(tmpPath)
		return "", &transpareo.Error{Code: CodeReplace, Message: err.Error()}
	}
	return target, nil
}
