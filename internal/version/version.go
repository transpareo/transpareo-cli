// Package version holds the build information the release
// process injects with -ldflags.
package version

import "runtime/debug"

// Set at build time by goreleaser; the defaults describe a
// build from source.
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// String returns the version with the commit when one is known.
func String() string {
	v := Version
	if v == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" &&
			info.Main.Version != "(devel)" {
			v = info.Main.Version
		}
	}
	if Commit != "" {
		short := Commit
		if len(short) > 12 {
			short = short[:12]
		}
		v += " (" + short + ")"
	}
	return v
}
