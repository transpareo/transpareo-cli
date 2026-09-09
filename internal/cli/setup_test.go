package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupClaudeInstallsSkillAndRegistersServer(t *testing.T) {
	h := newHarness(t)
	h.login()
	home := t.TempDir()
	h.env["HOME"] = home
	var ran []string
	stdout, stderr := "", ""
	run := func(args ...string) (string, string, int) {
		out, errOut, code := h.runWith(func(app *App) {
			app.LookPath = func(binary string) (string, error) {
				return "/usr/bin/" + binary, nil
			}
			app.Run = func(binary string, args ...string) error {
				ran = append([]string{binary}, args...)
				return nil
			}
		}, args...)
		stdout, stderr = out, errOut
		return out, errOut, code
	}
	if _, _, code := run("setup", "claude"); code != 0 {
		t.Fatalf("code %d: %s %s", code, stdout, stderr)
	}
	skill, err := os.ReadFile(filepath.Join(home, ".claude", "skills",
		"transpareo", "SKILL.md"))
	if err != nil ||
		!strings.HasPrefix(string(skill), "---\nname: transpareo") {
		t.Errorf("skill not installed: %v", err)
	}
	want := "/usr/bin/claude mcp add --scope user transpareo -- " +
		"transpareo mcp --profile " + profileNameFor(h.server.URL)
	if strings.Join(ran, " ") != want {
		t.Errorf("ran %q, want %q", strings.Join(ran, " "), want)
	}
	if !strings.Contains(stderr, "registered for Claude Code") {
		t.Errorf("stderr = %q", stderr)
	}

	ran = nil
	if _, _, code := run("setup", "codex", "--read-only",
		"--no-skill"); code != 0 {
		t.Fatalf("code %d: %s", code, stderr)
	}
	want = "/usr/bin/codex mcp add transpareo -- transpareo mcp --profile " +
		profileNameFor(h.server.URL) + " --read-only"
	if strings.Join(ran, " ") != want {
		t.Errorf("ran %q, want %q", strings.Join(ran, " "), want)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex")); err == nil {
		t.Error("--no-skill must not create the codex skills directory")
	}
}

func TestSetupPrintsSnippetWithoutTheAssistant(t *testing.T) {
	h := newHarness(t)
	h.env["HOME"] = t.TempDir()
	_, stderr, code := h.runWith(func(app *App) {
		app.LookPath = func(string) (string, error) {
			return "",
				errors.New("not found")
		}
	}, "setup", "codex", "--no-skill")
	if code != 0 || !strings.Contains(stderr, "[mcp_servers.transpareo]") ||
		!strings.Contains(stderr, `command = "transpareo"`) {
		t.Errorf("code %d, stderr %q", code, stderr)
	}
	_, stderr, _ = h.runWith(func(app *App) {
		app.LookPath = func(string) (string, error) {
			return "",
				errors.New("not found")
		}
	}, "setup", "claude", "--no-skill")
	if !strings.Contains(stderr, `"mcpServers"`) {
		t.Errorf("stderr = %q", stderr)
	}
	if _, _, code := h.run("setup", "cursor"); code != 2 {
		t.Errorf("unknown assistant: code %d", code)
	}
}
