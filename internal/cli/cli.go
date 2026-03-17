// Package cli implements the ix command surface (docs/DESIGN.md §4).
//
// This is the skeleton: the command router and signatures are defined so the
// shape is agreed, but the bodies return errNotImplemented until the
// corresponding build phase fills them in. A hand-rolled dispatch keeps the
// dependency graph empty for now; a richer flag library can be adopted when the
// commands gain real flags.
package cli

import (
	"context"
	"errors"
	"fmt"
)

var errNotImplemented = errors.New("not implemented yet (skeleton)")

type command struct {
	name    string
	summary string
	run     func(ctx context.Context, args []string) error
}

func commands() []command {
	return []command{
		{"init", "Scaffold a new project", cmdInit},
		{"add", "Vendor one or more blocks into the project", cmdAdd},
		{"list", "List available / installed blocks", cmdList},
		{"generate", "Run the codegen pipeline (atlas|sqlc|gqlgen|houdini|all)", cmdGenerate},
		{"migrate", "Manage Atlas migrations (new <name>)", cmdMigrate},
		{"upgrade", "Re-render blocks at the new version, 3-way merge into the tree", cmdUpgrade},
		{"diff", "Show what an upgrade would change, without writing", cmdDiff},
		{"doctor", "Verify toolchain + detect drift", cmdDoctor},
		{"version", "Print CLI + embedded registry version", cmdVersion},
	}
}

// Run dispatches argv[1:] to a subcommand.
func Run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage()
		return nil
	}
	name, rest := args[0], args[1:]
	for _, c := range commands() {
		if c.name == name {
			return c.run(ctx, rest)
		}
	}
	usage()
	return fmt.Errorf("unknown command %q", name)
}

func usage() {
	fmt.Println("ix — schema-first full-stack scaffolding\n\nUsage: ix <command> [args]\n\nCommands:")
	for _, c := range commands() {
		fmt.Printf("  %-10s %s\n", c.name, c.summary)
	}
}
