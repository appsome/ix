// Command ix is the scaffolding CLI for the ix framework.
//
// It scaffolds schema-first Go + SvelteKit projects and keeps the vendored
// parts upgradeable. See docs/DESIGN.md for the full design; this entrypoint
// only wires the command surface defined in §4.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/appsome/ix/internal/cli"
)

func main() {
	if err := cli.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ix: "+err.Error())
		os.Exit(1)
	}
}
