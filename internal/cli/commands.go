package cli

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/appsome/ix/internal/registry"
)

// Version is the CLI + registry version. Overridden at build time via
// -ldflags "-X github.com/appsome/ix/internal/cli.Version=vX.Y.Z".
var Version = "0.0.0-dev"

// logf prints a progress line to stdout.
func logf(format string, args ...any) { fmt.Printf(format+"\n", args...) }

// cmdInit scaffolds a new project: renders the base scaffold, writes ix.yaml +
// ix.lock, and installs the base block set.
func cmdInit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	module := fs.String("module", "", "Go module path for the new project (required)")
	postgis := fs.Bool("postgis", false, "enable the PostGIS extension + sqlc geometry override")
	goVersion := fs.String("go", "1.25.0", "go directive for the generated go.mod")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *module == "" {
		return fmt.Errorf("init: --module is required (e.g. --module gitlab.com/acme/widgets)")
	}
	dir := "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, registry.ManifestFile)); err == nil {
		return fmt.Errorf("init: %s already contains an %s", root, registry.ManifestFile)
	}

	logf("Scaffolding %s in %s", *module, root)
	vars := map[string]any{"Module": *module, "PostGIS": *postgis, "GoVersion": *goVersion}
	if err := renderScaffold(root, vars); err != nil {
		return err
	}

	manifest := registry.DefaultManifest(*module, *postgis)
	lock := &registry.Lockfile{Version: 1, CLIVersion: Version, RegistryVersion: registryVersion(), Blocks: map[string]registry.LockBlock{}}

	// Install the base block set: the schema pipeline + the HTTP/GraphQL
	// server. Both are runtime-free so the project builds before any glue
	// block (and its ix/runtime dependency) is added.
	for _, name := range []string{"core-schema", "graphql-server"} {
		if err := installBlock(root, manifest, lock, name, nil, false); err != nil {
			return err
		}
	}

	if err := registry.SaveManifest(root, manifest); err != nil {
		return err
	}
	if err := registry.SaveLock(root, lock); err != nil {
		return err
	}
	logf("\nDone. Next:\n  cd %s\n  cp .env.example .env\n  # edit internal/datastore/schema.sql, then:\n  ix add entity --name <thing>\n  ix generate", dir)
	return nil
}

// renderScaffold materializes templates/project into root.
func renderScaffold(root string, vars map[string]any) error {
	sub, files, err := registry.ProjectScaffold()
	if err != nil {
		return err
	}
	for _, rel := range files {
		data, err := fs.ReadFile(sub, rel)
		if err != nil {
			return err
		}
		dest := rel
		render := false
		if strings.HasSuffix(rel, ".tmpl") {
			dest = strings.TrimSuffix(rel, ".tmpl")
			render = true
		}
		out := data
		if render {
			out, err = registry.RenderTemplate(rel, data, vars)
			if err != nil {
				return err
			}
		}
		abs := filepath.Join(root, dest)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, out, 0o644); err != nil {
			return err
		}
		logf("  + %s", dest)
	}
	return nil
}

// cmdAdd vendors one or more blocks into the current project.
func cmdAdd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	name := fs.String("name", "", "entity name (for the `entity` generator block)")
	frontend := fs.Bool("frontend", false, "also scaffold frontend files (for `entity`)")
	noHooks := fs.Bool("no-hooks", false, "do not run post_add hooks")

	// Parse flags that may be interspersed with positional block names, so the
	// documented form `ix add entity --name product --frontend` works the same
	// as `ix add --name product --frontend entity` (Go's flag package otherwise
	// stops at the first non-flag arg).
	var blocks []string
	rest := args
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			return err
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		blocks = append(blocks, rest[0])
		rest = rest[1:]
	}
	if len(blocks) == 0 {
		return fmt.Errorf("add: expected at least one block name (see `ix list`)")
	}

	root, err := registry.FindProjectRoot(".")
	if err != nil {
		return err
	}
	manifest, err := registry.LoadManifest(root)
	if err != nil {
		return err
	}
	lock, err := registry.LoadLock(root)
	if err != nil {
		return err
	}

	params := map[string]any{}
	if *name != "" {
		for k, v := range entityInflections(*name) {
			params[k] = v
		}
	}
	if *frontend {
		params["Frontend"] = true
	}

	for _, block := range blocks {
		if err := installBlock(root, manifest, lock, block, params, !*noHooks); err != nil {
			return err
		}
	}
	if err := registry.SaveManifest(root, manifest); err != nil {
		return err
	}
	return registry.SaveLock(root, lock)
}

func installBlock(root string, m *registry.Manifest, lock *registry.Lockfile, name string, params map[string]any, runHooks bool) error {
	b, err := registry.LoadBlock(name)
	if err != nil {
		return err
	}
	logf("Adding %s@%s — %s", b.Name, b.Version, b.Summary)
	_, err = registry.Install(root, m, lock, b, registry.InstallOptions{
		Params:   params,
		RunHooks: runHooks,
		Logf:     logf,
	})
	return err
}

// cmdList prints available blocks, marking installed ones when run inside a
// project.
func cmdList(ctx context.Context, args []string) error {
	idx, err := registry.LoadIndex()
	if err != nil {
		return err
	}

	installed := map[string]string{} // name -> installed version
	if root, err := registry.FindProjectRoot("."); err == nil {
		if lock, err := registry.LoadLock(root); err == nil {
			for name, lb := range lock.Blocks {
				installed[name] = lb.Version
			}
		}
	}

	logf("Available blocks (registry %s):\n", idx.Version)
	for _, e := range idx.Blocks {
		mark := " "
		ver := e.Version
		if iv, ok := installed[e.Name]; ok {
			mark = "✓"
			if iv != e.Version {
				ver = fmt.Sprintf("%s (installed %s)", e.Version, iv)
			}
		}
		logf("  %s %-15s %-13s %s", mark, e.Name, ver, e.Summary)
	}
	if len(installed) > 0 {
		logf("\n✓ = installed in this project")
	}
	return nil
}

// cmdGenerate runs the codegen pipeline. With no arg it prefers the vendored
// scripts/generate.sh, falling back to sqlc + gqlgen.
func cmdGenerate(ctx context.Context, args []string) error {
	root, err := registry.FindProjectRoot(".")
	if err != nil {
		return err
	}
	target := "all"
	if len(args) > 0 {
		target = args[0]
	}

	switch target {
	case "sqlc":
		return run(root, "sqlc", "generate")
	case "gqlgen":
		return run(root, "go", "run", "github.com/99designs/gqlgen", "generate")
	case "all":
		if _, err := os.Stat(filepath.Join(root, "scripts/generate.sh")); err == nil {
			return run(root, "bash", "scripts/generate.sh")
		}
		if err := run(root, "sqlc", "generate"); err != nil {
			return err
		}
		return run(root, "go", "run", "github.com/99designs/gqlgen", "generate")
	default:
		return fmt.Errorf("generate: unknown target %q (want sqlc|gqlgen|all)", target)
	}
}

// cmdMigrate wraps Atlas: `ix migrate new <name>`.
func cmdMigrate(ctx context.Context, args []string) error {
	root, err := registry.FindProjectRoot(".")
	if err != nil {
		return err
	}
	if len(args) < 2 || args[0] != "new" {
		return fmt.Errorf("migrate: usage: ix migrate new <name>")
	}
	name := strings.ToLower(strings.ReplaceAll(args[1], " ", "_"))
	return run(root, "atlas", "migrate", "diff", name, "--env", "local")
}

func run(dir, name string, args ...string) error {
	logf("$ %s %s", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func registryVersion() string {
	if idx, err := registry.LoadIndex(); err == nil {
		return idx.Version
	}
	return "unknown"
}

func cmdUpgrade(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	noHooks := fs.Bool("no-hooks", false, "do not run post_upgrade hooks or `go get` for runtime bumps")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runUpgrade(false, fs.Args(), !*noHooks)
}

func cmdDiff(ctx context.Context, args []string) error {
	return runUpgrade(true, args, false)
}

// runUpgrade drives `ix upgrade` (dryRun=false) and `ix diff` (dryRun=true)
// across the requested blocks (or all installed blocks when none are named).
func runUpgrade(dryRun bool, blocks []string, runHooks bool) error {
	root, err := registry.FindProjectRoot(".")
	if err != nil {
		return err
	}
	manifest, err := registry.LoadManifest(root)
	if err != nil {
		return err
	}
	lock, err := registry.LoadLock(root)
	if err != nil {
		return err
	}

	targets := blocks
	if len(targets) == 0 {
		for _, b := range manifest.Blocks {
			targets = append(targets, b.Name)
		}
	}

	conflicts, changed := 0, 0
	for _, name := range targets {
		if _, ok := lock.Blocks[name]; !ok {
			logf("• %s — not installed, skipping", name)
			continue
		}
		b, err := registry.LoadBlock(name)
		if err != nil {
			return err
		}
		res, err := registry.Upgrade(root, manifest, lock, b, registry.UpgradeOptions{
			DryRun: dryRun, RunHooks: runHooks, Logf: logf,
		})
		if err != nil {
			return err
		}
		verb := "upgrade"
		if dryRun {
			verb = "diff"
		}
		header := fmt.Sprintf("• %s %s@%s", verb, res.Block, res.ToVersion)
		if res.FromVersion != "" && res.FromVersion != res.ToVersion {
			header = fmt.Sprintf("• %s %s %s→%s", verb, res.Block, res.FromVersion, res.ToVersion)
		}
		logf("%s", header)

		for _, f := range res.Files {
			switch f.Status {
			case registry.StatusUnchanged:
				// quiet — nothing to report
			case registry.StatusSkipped:
				if f.Dest != "" {
					logf("    - %-40s skipped (%s)", f.Dest, f.Note)
				}
			default:
				changed++
				if f.Status == registry.StatusConflict {
					conflicts++
				}
				logf("    %s %s", statusMark(f.Status), f.Dest)
				if dryRun && f.Diff != "" {
					logf("%s", indentDiff(f.Diff))
				}
			}
		}
		if res.RuntimeBumped {
			logf("    runtime %s → %s", res.RuntimeFrom, res.RuntimeTo)
		}
	}

	if !dryRun {
		if err := registry.SaveLock(root, lock); err != nil {
			return err
		}
	}

	switch {
	case dryRun && changed == 0:
		logf("\nUp to date — no changes from upstream.")
	case dryRun:
		logf("\n%d file(s) would change. Run `ix upgrade` to apply.", changed)
	case conflicts > 0:
		logf("\n⚠ %d conflict(s) — resolve the <<<<<<< markers, then commit.", conflicts)
	case changed > 0:
		logf("\n✓ Upgraded %d file(s) cleanly.", changed)
	default:
		logf("\nAlready up to date.")
	}
	return nil
}

func statusMark(s registry.FileStatus) string {
	switch s {
	case registry.StatusAdded:
		return "+"
	case registry.StatusMerged:
		return "~"
	case registry.StatusConflict:
		return "✗"
	default:
		return " "
	}
}

func indentDiff(diff string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(diff, "\n"), "\n") {
		b.WriteString("      " + line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func cmdDoctor(ctx context.Context, args []string) error {
	// Phase 4: check toolchain presence/versions + verify ix.lock hashes match
	// the working tree (drift) + runtime-version constraints.
	return fmt.Errorf("doctor: %w", errNotImplemented)
}

func cmdVersion(ctx context.Context, args []string) error {
	fmt.Printf("ix %s (registry %s)\n", Version, registryVersion())
	return nil
}
