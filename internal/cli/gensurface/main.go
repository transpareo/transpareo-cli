// Command gensurface writes surface.json, the snapshot of the
// command tree that the checks compare against.
package main

import (
	"log"
	"os"

	"github.com/transpareo/transpareo-cli/internal/cli"
)

func main() {
	app, err := cli.GeneratorApp()
	if err != nil {
		log.Fatal(err)
	}
	data, err := cli.SurfaceJSON(app.Root())
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("../../surface.json", data, 0o644); err != nil {
		log.Fatal(err)
	}
}
