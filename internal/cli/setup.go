package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/transpareo/transpareo-cli/internal/output"
	"github.com/transpareo/transpareo-cli/skills"
)

// assistant describes where an assistant keeps skills and how its
// command line registers an MCP server.
type assistant struct {
	name      string
	binary    string
	skillDir  func(home string) string
	mcpArgs   func(server []string) []string
	snippet   func(server []string) string
	storeNote string
}

var assistants = map[string]assistant{
	"claude": {
		name:   "Claude Code",
		binary: "claude",
		skillDir: func(home string) string {
			return filepath.Join(home, ".claude", "skills", "transpareo")
		},
		mcpArgs: func(server []string) []string {
			return append([]string{"mcp", "add", "--scope", "user",
				"transpareo", "--"}, server...)
		},
		snippet: func(server []string) string {
			data,
				_ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{
				"transpareo": map[string]any{"type": "stdio",
					"command": server[0],
					"args":    server[1:]}}}, "", "  ")
			return "Add to ~/.claude.json:\n" + string(data)
		},
		storeNote: "registered for Claude Code (user scope)",
	},
	"codex": {
		name:   "Codex",
		binary: "codex",
		skillDir: func(home string) string {
			return filepath.Join(home, ".codex", "skills", "transpareo")
		},
		mcpArgs: func(server []string) []string {
			return append([]string{"mcp", "add", "transpareo", "--"}, server...)
		},
		snippet: func(server []string) string {
			args := make([]string, len(server)-1)
			for i, a := range server[1:] {
				args[i] = fmt.Sprintf("%q", a)
			}
			return "Add to ~/.codex/config.toml:\n[mcp_servers.transpareo]\n" +
				fmt.Sprintf("command = %q\nargs = [%s]", server[0],
					strings.Join(args, ", "))
		},
		storeNote: "registered for Codex",
	},
}

func (a *App) setupCommand() *cobra.Command {
	var noMCP, noSkill bool
	cmd := &cobra.Command{
		Use: "setup <assistant>",
		Short: "Install the skill and register the MCP server for " +
			"claude or codex",
		Long: `Copies the transpareo skill into the assistant's skills directory and
registers the MCP server for the selected profile through the
assistant's own command line, so the registration holds no
secret. When the assistant's command is not installed, the
configuration snippet is printed instead.`,
		Example: "  transpareo setup claude --profile acme\n" +
			"  transpareo setup codex",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"claude", "codex"},
		RunE: func(cmd *cobra.Command, args []string) error {
			target, ok := assistants[args[0]]
			if !ok {
				return output.Exit(output.ExitUsage,
					fmt.Errorf("unknown assistant %q; use claude or codex",
						args[0]))
			}
			return a.setup(target, noMCP, noSkill)
		},
	}
	cmd.Flags().BoolVar(&noMCP, "no-mcp", false, "install the skill only")
	cmd.Flags().BoolVar(&noSkill, "no-skill", false,
		"register the MCP server only")
	return cmd
}

func (a *App) setup(target assistant, noMCP, noSkill bool) error {
	printer := a.Printer()
	home := a.Getenv("HOME")
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return err
		}
	}
	if !noSkill {
		dir := target.skillDir(home)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		path := filepath.Join(dir, "SKILL.md")
		if err := os.WriteFile(path, skills.Transpareo, 0o644); err != nil {
			return err
		}
		printer.Message("Skill installed at %s.", path)
	}
	if noMCP {
		return nil
	}
	server := []string{"transpareo", "mcp"}
	name, _, err := a.Resolver().ProfileName(a.Profile)
	if err != nil {
		return err
	}
	if name != "" {
		server = append(server, "--profile", name)
	}
	if a.ReadOnly {
		server = append(server, "--read-only")
	}
	binary, err := a.lookPath(target.binary)
	if err != nil {
		printer.Message("%s is not on the PATH, so the server was not"+
			"registered. %s",
			target.binary, target.snippet(server))
		return nil
	}
	if err := a.run(binary, target.mcpArgs(server)...); err != nil {
		return fmt.Errorf("%s: %w", target.name, err)
	}
	printer.Message("MCP server %s: %s.", target.storeNote, strings.Join(server,
		" "))
	return nil
}

// lookPath and run are replaced in tests.
func (a *App) lookPath(binary string) (string, error) {
	if a.LookPath != nil {
		return a.LookPath(binary)
	}
	return exec.LookPath(binary)
}

func (a *App) run(binary string, args ...string) error {
	if a.Run != nil {
		return a.Run(binary, args...)
	}
	cmd := exec.Command(binary, args...)
	cmd.Stdout = a.Stderr
	cmd.Stderr = a.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return fmt.Errorf("%s exited with %d", binary, exit.ExitCode())
		}
		return err
	}
	return nil
}
