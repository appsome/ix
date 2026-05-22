//go:build genvalidate

package genvalidate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each test below assembles the project independently (t.TempDir is per-test)
// and feeds one class of generated file to its real consuming tool. They share
// no mutable state, so `go test -tags genvalidate -run TestValidate_Sqlc` runs
// any single validator in isolation.

// TestValidate_Go feeds every generated .go file to gofmt, which parses and
// canonically formats Go. Any file gofmt would rewrite is not clean — that
// catches both syntax garbage and template-induced misindentation (e.g. a
// patch that inserts space-indented code into a tab-indented file).
func TestValidate_Go(t *testing.T) {
	requireTool(t, "gofmt")
	p := assembleProject(t)
	// gofmt -l lists files that differ from gofmt output; empty == all clean.
	out, err := runTool(p.root, "gofmt", "-l", ".")
	if err != nil {
		t.Fatalf("gofmt: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "" {
		// Show the actual diffs to make the failure actionable.
		diff, _ := runTool(p.root, "gofmt", "-d", ".")
		t.Errorf("generated Go is not gofmt-clean:\n%s\n--- diffs ---\n%s", out, diff)
	}
}

// TestValidate_Sqlc runs `sqlc compile`, which parses sqlc.yaml, the schema,
// and every query, resolving each query against the schema (including the
// PostGIS geometry type override). It is the tool that actually consumes these
// files in `ix generate`.
func TestValidate_Sqlc(t *testing.T) {
	requireTool(t, "sqlc")
	p := assembleProject(t)
	out, err := runTool(p.root, "sqlc", "compile")
	if err != nil {
		t.Errorf("sqlc compile failed:\n%s", out)
	}
}

// TestValidate_Atlas validates the generated atlas.hcl by loading the `local`
// env and validating the migration directory through it — exercising the HCL
// parse, the env block, and the format.migrate.diff directive whose escaping
// was previously broken.
func TestValidate_Atlas(t *testing.T) {
	requireTool(t, "atlas")
	p := assembleProject(t)
	// A fresh migration dir needs its checksum (atlas.sum) before validate.
	if out, err := runTool(p.root, "atlas", "migrate", "hash"); err != nil {
		t.Fatalf("atlas migrate hash:\n%s", out)
	}
	out, err := runTool(p.root, "atlas", "migrate", "validate", "--env", "local")
	if err != nil {
		t.Errorf("atlas migrate validate failed (atlas.hcl may be malformed):\n%s", out)
	}
}

// TestValidate_Helm lints the generated umbrella chart and renders it. `helm
// lint` checks Chart.yaml + values + template structure; `helm template`
// proves every template actually renders to valid YAML manifests.
func TestValidate_Helm(t *testing.T) {
	requireTool(t, "helm")
	p := assembleProject(t)
	chart := chartDir(t, p.root)
	if out, err := runTool(p.root, "helm", "lint", chart); err != nil {
		t.Errorf("helm lint failed:\n%s", out)
	}
	if out, err := runTool(p.root, "helm", "template", chart); err != nil {
		t.Errorf("helm template failed to render:\n%s", out)
	}
}

// TestValidate_Compose validates docker-compose.yml with `docker compose
// config`, which parses and resolves the whole compose file (services,
// volumes, interpolation). It does not need the Docker daemon.
func TestValidate_Compose(t *testing.T) {
	requireTool(t, "docker")
	p := assembleProject(t)
	out, err := runTool(p.root, "docker", "compose", "-f", "docker-compose.yml", "config", "-q")
	if err != nil {
		t.Errorf("docker compose config failed:\n%s", out)
	}
}

// TestValidate_PreCommit validates the generated .pre-commit-config.yaml with
// pre-commit's own config validator.
func TestValidate_PreCommit(t *testing.T) {
	requireTool(t, "pre-commit")
	p := assembleProject(t)
	cfg := filepath.Join(p.root, ".pre-commit-config.yaml")
	if _, err := os.Stat(cfg); err != nil {
		t.Skipf("no .pre-commit-config.yaml generated: %v", err)
	}
	out, err := runTool(p.root, "pre-commit", "validate-config", cfg)
	if err != nil {
		t.Errorf("pre-commit validate-config failed:\n%s", out)
	}
}

// TestValidate_JSON feeds every generated .json file to jq, which fully parses
// JSON.
func TestValidate_JSON(t *testing.T) {
	requireTool(t, "jq")
	p := assembleProject(t)
	files := globGenerated(t, p.root, ".json")
	if len(files) == 0 {
		t.Skip("no .json files generated")
	}
	for _, f := range files {
		if out, err := runTool(p.root, "jq", "-e", ".", f); err != nil {
			t.Errorf("%s: not valid JSON:\n%s", rel(p.root, f), out)
		}
	}
}

// TestValidate_YAML feeds every generated YAML file to yq, which fully parses
// YAML. Helm chart *templates* are skipped — they contain Helm directives and
// are validated by TestValidate_Helm instead.
func TestValidate_YAML(t *testing.T) {
	requireTool(t, "yq")
	p := assembleProject(t)
	var files []string
	files = append(files, globGenerated(t, p.root, ".yml")...)
	files = append(files, globGenerated(t, p.root, ".yaml")...)
	if len(files) == 0 {
		t.Skip("no YAML files generated")
	}
	for _, f := range files {
		// Skip Helm template bodies (charts/*/templates/*.yaml): they are Go/Helm
		// templates, not plain YAML. helm template validates those.
		if strings.Contains(filepath.ToSlash(f), "/templates/") && strings.Contains(filepath.ToSlash(f), "/charts/") {
			continue
		}
		if out, err := runTool(p.root, "yq", "-e", ".", f); err != nil {
			t.Errorf("%s: not valid YAML:\n%s", rel(p.root, f), out)
		}
	}
}

// TestValidate_GenerateAndBuild runs the whole pipeline a generated project
// actually executes — sqlc → gqlgen → `go build ./...` — proving the generated
// Go, sqlc, and GraphQL artifacts not only parse individually but compile
// together into a working binary.
//
// It needs the network (go get / go mod tidy pull gqlgen + the runtime's
// transitive deps) and the local runtime module, which it wires in with a
// `replace` since runtime/v0.1.0 isn't published yet. It skips when go, sqlc,
// or the network are unavailable.
func TestValidate_GenerateAndBuild(t *testing.T) {
	requireTool(t, "go")
	requireTool(t, "sqlc")
	p := assembleProject(t)

	runtimeDir := filepath.Join(p.moduleRoot, "runtime")
	if _, err := os.Stat(filepath.Join(runtimeDir, "go.mod")); err != nil {
		t.Skipf("local runtime module not found at %s: %v", runtimeDir, err)
	}

	// Point the unpublished runtime at the local module. Blocks pin different
	// runtime tags (v0.1.0 for the original packages, v0.2.0 for jobs); a
	// version-less replace applies to every required version, so the local
	// tree satisfies all pins regardless of which tags are published yet.
	const runtimeMod = "github.com/appsome/ix/runtime"
	if out, err := runTool(p.root, "go", "mod", "edit",
		"-require="+runtimeMod+"@v0.2.0",
		"-replace="+runtimeMod+"="+runtimeDir); err != nil {
		t.Fatalf("go mod edit: %v\n%s", err, out)
	}

	// 1. sqlc: schema.sql + queries → internal/datastore/pkg/db.
	if out, err := runTool(p.root, "sqlc", "generate"); err != nil {
		t.Fatalf("sqlc generate:\n%s", out)
	}

	// 2. add the gqlgen tool dep, then generate the server scaffold. This is the
	// first command that hits the network; treat a network failure as a skip,
	// not a test failure, so offline machines stay green.
	if out, err := runTool(p.root, "go", "get", "github.com/99designs/gqlgen"); err != nil {
		if isNetworkErr(out) {
			t.Skipf("network unavailable (go get gqlgen); skipping build stage:\n%s", out)
		}
		t.Fatalf("go get gqlgen:\n%s", out)
	}
	if out, err := runTool(p.root, "go", "run", "github.com/99designs/gqlgen", "generate"); err != nil {
		if isNetworkErr(out) {
			t.Skipf("network unavailable (gqlgen generate); skipping build stage:\n%s", out)
		}
		t.Fatalf("gqlgen generate:\n%s", out)
	}

	// 3. resolve all deps now that generated packages exist.
	if out, err := runTool(p.root, "go", "mod", "tidy"); err != nil {
		if isNetworkErr(out) {
			t.Skipf("network unavailable (go mod tidy); skipping build stage:\n%s", out)
		}
		t.Fatalf("go mod tidy:\n%s", out)
	}

	// 4. the payoff: the whole generated project compiles.
	if out, err := runTool(p.root, "go", "build", "./..."); err != nil {
		t.Errorf("go build ./... failed on the generated project:\n%s", out)
	}
}

func isNetworkErr(out string) bool {
	for _, sig := range []string{
		"connection reset", "no such host", "i/o timeout",
		"dial tcp", "TLS handshake", "network is unreachable",
		"could not connect", "proxyconnect",
	} {
		if strings.Contains(out, sig) {
			return true
		}
	}
	return false
}

// chartDir finds the single generated Helm chart under charts/.
func chartDir(t *testing.T, root string) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "charts"))
	if err != nil {
		t.Fatalf("read charts/: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			return filepath.Join(root, "charts", e.Name())
		}
	}
	t.Fatal("no chart directory under charts/")
	return ""
}

// globGenerated walks the project tree and returns files with the given
// extension, excluding the .ix baseline (those are byte-identical copies).
func globGenerated(t *testing.T, root, ext string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".ix" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ext) {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

func rel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return r
}
