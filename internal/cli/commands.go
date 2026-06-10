package cli

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

// cmdGenerate runs the codegen pipeline. With no arg it runs `all`, preferring
// the vendored scripts/generate.sh and falling back to sqlc + gqlgen
// (+ houdini when the frontend exists). `atlas` is explicit-only — it diffs
// schema.sql into a new migration, which needs the dev database running and is
// not a no-op-on-rerun generator like the others.
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
	case "atlas":
		return run(root, "atlas", "migrate", "diff", "--env", "local")
	case "sqlc":
		return run(root, "sqlc", "generate")
	case "gqlgen":
		return run(root, "go", "run", "github.com/99designs/gqlgen", "generate")
	case "houdini":
		web, err := webDir(root)
		if err != nil {
			return err
		}
		return run(web, "npx", "houdini", "generate")
	case "all":
		if _, err := os.Stat(filepath.Join(root, "scripts/generate.sh")); err == nil {
			return run(root, "bash", "scripts/generate.sh")
		}
		if err := run(root, "sqlc", "generate"); err != nil {
			return err
		}
		if err := run(root, "go", "run", "github.com/99designs/gqlgen", "generate"); err != nil {
			return err
		}
		if web, err := webDir(root); err == nil {
			return run(web, "npx", "houdini", "generate")
		}
		return nil
	default:
		return fmt.Errorf("generate: unknown target %q (want atlas|sqlc|gqlgen|houdini|all)", target)
	}
}

// webDir resolves the project's frontend root (ix.yaml paths.web, default
// "web") and verifies Houdini is configured there.
func webDir(root string) (string, error) {
	web := "web"
	if m, err := registry.LoadManifest(root); err == nil && m.Paths.Web != "" {
		web = m.Paths.Web
	}
	dir := filepath.Join(root, web)
	if _, err := os.Stat(filepath.Join(dir, "houdini.config.js")); err != nil {
		return "", fmt.Errorf("generate houdini: no houdini.config.js in %s/ — add the frontend first (`ix add admin-svelte`)", web)
	}
	return dir, nil
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

// cmdDoctor verifies the toolchain on PATH and, when run inside a project, the
// ix-managed state: manifest↔lock consistency, working-tree drift against
// ix.lock, runtime version constraints, and pending block updates
// (docs/DESIGN.md §4 and §10). Exits non-zero when problems are found; local
// edits to managed files are reported but are not problems — that is the
// vendored model working as intended.
func cmdDoctor(ctx context.Context, args []string) error {
	problems := 0
	fail := func(format string, args ...any) {
		problems++
		logf("  ✗ "+format, args...)
	}

	root, rootErr := registry.FindProjectRoot(".")
	var manifest *registry.Manifest
	if rootErr == nil {
		var err error
		if manifest, err = registry.LoadManifest(root); err != nil {
			return err
		}
	}

	// Toolchain. node only becomes required once the project has a frontend.
	needNode := manifest != nil && manifest.Frontend.Framework != ""
	logf("Toolchain:")
	for _, t := range []struct {
		name     string
		args     []string
		required bool
		usedFor  string
	}{
		{"go", []string{"version"}, true, "building the project and running gqlgen"},
		{"sqlc", []string{"version"}, true, "ix generate"},
		{"atlas", []string{"version"}, true, "ix migrate and migration diffing"},
		{"node", []string{"--version"}, needNode, "the frontend build"},
		{"docker", []string{"--version"}, false, "the compose dev environment"},
	} {
		version, found := toolVersion(ctx, t.name, t.args...)
		switch {
		case found:
			logf("  ✓ %-8s %s", t.name, version)
		case t.required:
			fail("%-8s not found on PATH — needed for %s", t.name, t.usedFor)
		default:
			logf("  - %-8s not found (optional — %s)", t.name, t.usedFor)
		}
	}

	if rootErr != nil {
		logf("\nNot inside an ix project (no %s) — project checks skipped.", registry.ManifestFile)
		return doctorVerdict(problems)
	}

	lock, err := registry.LoadLock(root)
	if err != nil {
		return err
	}
	lockNames := slices.Sorted(maps.Keys(lock.Blocks))

	// Manifest ↔ lock consistency.
	logf("\nProject %s:", root)
	for _, b := range manifest.Blocks {
		if _, ok := lock.Blocks[b.Name]; !ok {
			fail("%s listed in %s but missing from %s — re-run `ix add %s`",
				b.Name, registry.ManifestFile, registry.LockFileName, b.Name)
		}
	}
	for _, name := range lockNames {
		if !slices.ContainsFunc(manifest.Blocks, func(b registry.InstalledBlock) bool { return b.Name == name }) {
			fail("%s locked in %s but absent from %s — re-run `ix add %s` or remove the lock entry",
				name, registry.LockFileName, registry.ManifestFile, name)
		}
	}

	// Working-tree drift against the lock's pristine-render hashes.
	clean, edited := 0, 0
	for _, d := range registry.CheckDrift(root, lock) {
		switch d.Status {
		case registry.DriftClean:
			clean++
		case registry.DriftModified:
			edited++
			logf("  ~ %-40s local edits (%s) — `ix upgrade` will 3-way merge", d.Dest, d.Block)
		case registry.DriftMissing:
			fail("%-40s tracked by %s (%s) but missing from the tree", d.Dest, registry.LockFileName, d.Block)
		case registry.DriftNoBaseline:
			fail("%-40s has no .ix/baseline copy (%s) — upgrades cannot merge it", d.Dest, d.Block)
		}
	}
	logf("  ✓ managed files: %d pristine, %d locally edited", clean, edited)

	// Runtime constraint: go.mod must require at least the newest version any
	// installed block needs (docs/DESIGN.md §10).
	type runtimeReq struct{ version, by string }
	reqs := map[string]runtimeReq{}
	for _, name := range lockNames {
		rt := lock.Blocks[name].Runtime
		if rt == nil || rt.Version == "" {
			continue
		}
		if r, ok := reqs[rt.Module]; !ok || registry.CompareVersions(rt.Version, r.version) > 0 {
			reqs[rt.Module] = runtimeReq{rt.Version, name}
		}
	}
	for _, module := range slices.Sorted(maps.Keys(reqs)) {
		req := reqs[module]
		have, replaced, err := registry.GoModRequire(root, module)
		switch {
		case err != nil:
			fail("%v — %s requires %s %s", err, req.by, module, req.version)
		case have == "":
			fail("%s requires %s %s but go.mod does not require it — run `go get %s@%s`",
				req.by, module, req.version, module, req.version)
		case replaced:
			logf("  - %s is `replace`d in go.mod — version check skipped", module)
		case registry.CompareVersions(have, req.version) < 0:
			fail("go.mod has %s %s but %s requires ≥ %s — run `go get %s@%s`",
				module, have, req.by, req.version, module, req.version)
		default:
			logf("  ✓ runtime %s %s satisfies all installed blocks", module, have)
		}
	}

	// Pending updates against this CLI's embedded registry.
	updates := 0
	for _, name := range lockNames {
		b, err := registry.LoadBlock(name)
		if err != nil {
			continue // not in this CLI's registry (e.g. placeholder block)
		}
		if installed := lock.Blocks[name].Version; registry.CompareVersions(b.Version, installed) > 0 {
			updates++
			logf("  ↑ %-40s %s → %s available — `ix upgrade %s`", name, installed, b.Version, name)
		}
	}
	if updates == 0 {
		logf("  ✓ all installed blocks match the embedded registry (%s)", registryVersion())
	}

	return doctorVerdict(problems)
}

func doctorVerdict(problems int) error {
	if problems == 0 {
		logf("\n✓ No problems found.")
		return nil
	}
	return fmt.Errorf("doctor: %d problem(s) found", problems)
}

// toolVersion reports whether name is on PATH and, if so, the first line of
// its version output (best-effort: a tool that is present but fails the
// version probe still counts as found).
func toolVersion(ctx context.Context, name string, args ...string) (string, bool) {
	if _, err := exec.LookPath(name); err != nil {
		return "", false
	}
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return "(version unknown)", true
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return strings.TrimSpace(line), true
}

func cmdVersion(ctx context.Context, args []string) error {
	fmt.Printf("ix %s (registry %s)\n", Version, registryVersion())
	return nil
}
