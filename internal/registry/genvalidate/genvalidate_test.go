//go:build genvalidate

// Package genvalidate holds heavy, tool-backed validation of the files ix
// generates. It assembles a full project the way a user does — build the ix
// binary, `ix init`, then `ix add` the real blocks — and then feeds each
// generated artifact to its REAL consuming tool (gofmt, sqlc, atlas, helm,
// docker compose, pre-commit, jq, yq) rather than to a hand-rolled heuristic.
// If the tool that actually reads the file accepts it, it isn't garbage.
//
// These tests are gated behind the `genvalidate` build tag so the default
// `go test ./...` stays fast and hermetic:
//
//	go test -tags genvalidate ./internal/registry/genvalidate/
//
// Each tool-backed check probes for its binary on PATH and t.Skip()s when it's
// absent, so the suite runs partially on a bare machine and fully in CI once
// the toolchain is installed.
package genvalidate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// assembled is a project tree built once and shared across the validators.
type assembled struct {
	ix         string // path to the built ix binary
	root       string // project root (contains ix.yaml)
	moduleRoot string // ix repo root (holds runtime/ for the local replace)
}

// build compiles the ix binary into a temp dir. Fatal on failure — without it
// nothing else can run.
func buildIX(t *testing.T) (bin, moduleRoot string) {
	t.Helper()
	bin = filepath.Join(t.TempDir(), "ix")
	// The module root is three levels up from this package
	// (internal/registry/genvalidate).
	moduleRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/ix")
	cmd.Dir = moduleRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build ix: %v\n%s", err, out)
	}
	return bin, moduleRoot
}

// assembleProject runs `ix init` + `ix add` for every real block, mirroring the
// documented user flow, and returns the project root. The entity slice is made
// coherent by appending the `products` table the generated queries assume, so
// `sqlc compile` has a schema to resolve against.
func assembleProject(t *testing.T) *assembled {
	t.Helper()
	ix, moduleRoot := buildIX(t)
	root := filepath.Join(t.TempDir(), "proj")

	const module = "example.com/acme/widgets"
	runIX(t, ix, "", "init", "--module", module, "--postgis", root)

	runIX(t, ix, root, "add", "entity", "--name", "product", "--frontend")

	// Make the entity slice coherent for sqlc: add the table its queries assume.
	appendProductsTable(t, root)

	for _, block := range []string{
		"authz", "auth-jwt", "metrics", "pubsub", "jobs", "admin-cli",
		"docker", "compose", "helm",
		"ci-github", "ci-gitlab", "admin-svelte",
		// jobs-admin requires both jobs and admin-svelte, so it comes last.
		"jobs-admin",
	} {
		runIX(t, ix, root, "add", block)
	}
	return &assembled{ix: ix, root: root, moduleRoot: moduleRoot}
}

func runIX(t *testing.T, ix, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(ix, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ix %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func appendProductsTable(t *testing.T, root string) {
	t.Helper()
	schema := filepath.Join(root, "internal/datastore/schema.sql")
	b, err := os.ReadFile(schema)
	if err != nil {
		t.Fatalf("read schema.sql: %v", err)
	}
	const table = "\n\nCREATE TABLE products (\n" +
		"    id   bigserial PRIMARY KEY,\n" +
		"    name text NOT NULL\n);\n"
	if err := os.WriteFile(schema, append(b, table...), 0o644); err != nil {
		t.Fatalf("write schema.sql: %v", err)
	}
}

// requireTool skips the test if the named binary isn't on PATH.
func requireTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not on PATH; skipping (CI installs it for full coverage)", name)
	}
}

// runTool runs name+args in dir and returns combined output + error.
func runTool(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
