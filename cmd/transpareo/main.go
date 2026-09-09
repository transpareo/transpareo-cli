// Command transpareo is the command-line tool for the Transpareo
// API.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/transpareo/transpareo-cli/internal/cli"
)

func main() {
	app, err := cli.FromOS()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(cli.Main(ctx, app, os.Args[1:]))
}
