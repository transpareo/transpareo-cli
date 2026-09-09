// Command fetchroot downloads the Sigstore public-good trusted
// root through TUF and writes it beside the upgrade package, so
// that verification needs no network call besides the release
// download. Run it when Sigstore rotates its keys.
package main

import (
	"log"
	"os"

	"github.com/sigstore/sigstore-go/pkg/root"
)

func main() {
	trusted, err := root.FetchTrustedRoot()
	if err != nil {
		log.Fatal(err)
	}
	data, err := trusted.MarshalJSON()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("../trusted_root.json", append(data, '\n'),
		0o644); err != nil {
		log.Fatal(err)
	}
}
