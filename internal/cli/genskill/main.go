// Command genskill fills the reference section of the skill with
// every command of the tree, between the reference markers.
package main

import (
	"log"
	"os"
	"strings"

	"github.com/transpareo/transpareo-cli/internal/cli"
)

const path = "../../skills/transpareo/SKILL.md"

func main() {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	app, err := cli.GeneratorApp()
	if err != nil {
		log.Fatal(err)
	}
	updated, err := app.FillReference(string(data), app.Root())
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		log.Fatal(err)
	}
	if !strings.Contains(updated, cli.ReferenceStart) {
		log.Fatal("the skill lost its reference markers")
	}
}
