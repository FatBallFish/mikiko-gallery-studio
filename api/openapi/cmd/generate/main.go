// Command generate updates the authoritative OpenAPI and native JSON schemas.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/fatballfish/pic-gallery/api/openapi/schemagen"
	"os"
	"path/filepath"
)

func main() {
	check := flag.Bool("check", false, "fail if generated contracts differ")
	root := flag.String("root", "api/openapi", "authoritative OpenAPI directory")
	flag.Parse()
	path := filepath.Join(*root, "openapi.yaml")
	raw, err := os.ReadFile(path)
	must(err)
	updated, err := schemagen.Sync(raw)
	must(err)
	schemas, err := schemagen.Schemas()
	must(err)
	for name, data := range map[string][]byte{path: updated, filepath.Join(*root, "components/schemas/native.generated.json"): schemas} {
		if *check {
			current, err := os.ReadFile(name)
			must(err)
			if !bytes.Equal(current, data) {
				must(fmt.Errorf("%s is stale; run go run ./api/openapi/cmd/generate", name))
			}
		} else {
			must(os.WriteFile(name, data, 0644))
		}
	}
}
func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
