// Command gendocs writes docs/cli.md from the command tree: one
// section per group, every command with its example and the
// permission it needs.
package main

import (
	"log"
	"os"

	"github.com/transpareo/transpareo-cli/internal/cli"
)

func main() {
	app := &cli.App{Getenv: func(string) string { return "" }}
	doc, err := cli.Markdown(app.Root())
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("../../docs/cli.md", []byte(doc), 0o644); err != nil {
		log.Fatal(err)
	}
}
