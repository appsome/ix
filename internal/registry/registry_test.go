package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadIndexAndBlocks(t *testing.T) {
	idx, err := LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	if len(idx.Blocks) == 0 {
		t.Fatal("registry index is empty")
	}
	// Every indexed block must have a loadable block.yaml.
	for _, e := range idx.Blocks {
		b, err := LoadBlock(e.Name)
		if err != nil {
			// Some blocks are still placeholders (README only); skip those.
			if strings.Contains(err.Error(), "no block.yaml") {
				continue
			}
			t.Errorf("LoadBlock(%q): %v", e.Name, err)
			continue
		}
		if b.Name != e.Name {
			t.Errorf("block %q name mismatch: %q", e.Name, b.Name)
		}
	}
}

func TestInstall_CoreSchema(t *testing.T) {
	root := t.TempDir()
	m := DefaultManifest("gitlab.com/acme/widgets", true)
	lock := &Lockfile{Version: 1, Blocks: map[string]LockBlock{}}

	b, err := LoadBlock("core-schema")
	if err != nil {
		t.Fatalf("LoadBlock: %v", err)
	}
	res, err := Install(root, m, lock, b, InstallOptions{Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(res.Written) == 0 {
		t.Fatal("nothing written")
	}

	// schema.sql exists, rendered with the PostGIS branch + module path. The
	// branch deliberately does NOT emit `CREATE EXTENSION` — Atlas's
	// open-source CLI cannot diff extensions, so PostGIS is provided by the
	// dev image instead (see atlas.hcl). The branch renders an explanatory
	// comment we assert on here.
	schema, err := os.ReadFile(filepath.Join(root, "internal/datastore/schema.sql"))
	if err != nil {
		t.Fatalf("read schema.sql: %v", err)
	}
	if !strings.Contains(string(schema), "PostGIS (enabled because database.postgis = true") {
		t.Error("PostGIS branch not rendered despite postgis=true")
	}
	if strings.Contains(string(schema), "CREATE EXTENSION IF NOT EXISTS postgis;") {
		t.Error("schema.sql must not declare extensions (Atlas OSS cannot diff them)")
	}
	if !strings.Contains(string(schema), "gitlab.com/acme/widgets") {
		t.Error("module var not interpolated into schema.sql")
	}

	// atlas.hcl points the dev database at the PostGIS image so geometry types
	// resolve during diffing without managing the extension.
	atlasHCL, err := os.ReadFile(filepath.Join(root, "atlas.hcl"))
	if err != nil {
		t.Fatalf("read atlas.hcl: %v", err)
	}
	if !strings.Contains(string(atlasHCL), "imresamu/postgis") {
		t.Error("atlas.hcl PostGIS branch not rendered: expected the postgis dev image")
	}

	// Lock records every written file with a sha256 hash; baseline copies exist.
	lb, ok := lock.Blocks["core-schema"]
	if !ok {
		t.Fatal("lock missing core-schema entry")
	}
	for dest, lf := range lb.Files {
		if !strings.HasPrefix(lf.Hash, "sha256:") {
			t.Errorf("%s: bad hash %q", dest, lf.Hash)
		}
		if lf.Managed {
			if _, err := os.Stat(filepath.Join(root, baselineDir, dest)); err != nil {
				t.Errorf("%s: missing baseline copy", dest)
			}
		}
	}

	// Manifest updated.
	if !manifestHasBlock(m, "core-schema") {
		t.Error("manifest not updated with core-schema")
	}

	// Re-install is idempotent: existing files are skipped, not clobbered.
	res2, err := Install(root, m, lock, b, InstallOptions{Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatalf("re-Install: %v", err)
	}
	if len(res2.Written) != 0 {
		t.Errorf("re-install wrote %v, want nothing", res2.Written)
	}
}

func TestApplyPatch_ExactAnchorIgnoresProse(t *testing.T) {
	root := t.TempDir()
	target := "main.go"
	// The doc comment mentions the anchor token in prose; the real anchor is a
	// standalone marker line. The patch must land at the marker, not the prose.
	content := `package main

// mentions the // ix:wire-services anchor in prose
func main() {
	// ix:wire-services
	println("hi")
}
`
	if err := os.WriteFile(filepath.Join(root, target), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Patch{File: target, Anchor: "// ix:wire-services", Insert: "wired := true"}
	applied, err := applyPatch(root, p, map[string]any{})
	if err != nil {
		t.Fatalf("applyPatch: %v", err)
	}
	if !applied {
		t.Fatal("expected patch to apply")
	}

	got, _ := os.ReadFile(filepath.Join(root, target))
	lines := strings.Split(string(got), "\n")
	// "wired := true" must appear immediately after the marker line (inside main),
	// and exactly once.
	var markerIdx, insertIdx, count int
	for i, l := range lines {
		if strings.TrimSpace(l) == "// ix:wire-services" {
			markerIdx = i
		}
		if strings.Contains(l, "wired := true") {
			insertIdx = i
			count++
		}
	}
	if count != 1 {
		t.Fatalf("insert appears %d times, want 1", count)
	}
	if insertIdx != markerIdx+1 {
		t.Errorf("insert at line %d, want right after marker at %d", insertIdx, markerIdx)
	}
	// The insert must be indented to match the marker (one tab).
	if !strings.HasPrefix(lines[insertIdx], "\t") {
		t.Errorf("insert not reindented to marker: %q", lines[insertIdx])
	}

	// Idempotent.
	applied2, err := applyPatch(root, p, map[string]any{})
	if err != nil {
		t.Fatalf("re-applyPatch: %v", err)
	}
	if applied2 {
		t.Error("second apply should be a no-op")
	}
}

func TestUpgrade_MergesUpstreamWithLocalEdits(t *testing.T) {
	root := t.TempDir()
	m := DefaultManifest("gitlab.com/acme/widgets", false)
	lock := &Lockfile{Version: 1, Blocks: map[string]LockBlock{}}
	b, err := LoadBlock("core-schema")
	if err != nil {
		t.Fatalf("LoadBlock: %v", err)
	}
	if _, err := Install(root, m, lock, b, InstallOptions{Logf: func(string, ...any) {}}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	sqlcPath := filepath.Join(root, "sqlc.yaml")
	basePath := filepath.Join(root, baselineDir, "sqlc.yaml")

	// Simulate: upstream (the current template, == OURS) ADDED the
	// emit_empty_slices line — so the old baseline lacks it. The user
	// independently renamed the generated package in their working copy.
	dropLine(t, basePath, "emit_empty_slices: true")
	dropLine(t, sqlcPath, "emit_empty_slices: true")
	replaceInFile(t, sqlcPath, `package: "db"`, `package: "store"`)

	res, err := Upgrade(root, m, lock, b, UpgradeOptions{Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if st := fileStatus(res, "sqlc.yaml"); st != StatusMerged {
		t.Fatalf("sqlc.yaml status = %q, want merged", st)
	}

	got, _ := os.ReadFile(sqlcPath)
	if !strings.Contains(string(got), `package: "store"`) {
		t.Error("merge lost the local package rename")
	}
	if !strings.Contains(string(got), "emit_empty_slices: true") {
		t.Error("merge lost the upstream-added line")
	}

	// Lock version is set and a second upgrade is a no-op (baseline advanced).
	res2, err := Upgrade(root, m, lock, b, UpgradeOptions{Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatalf("re-Upgrade: %v", err)
	}
	for _, f := range res2.Files {
		if f.Status == StatusMerged || f.Status == StatusConflict || f.Status == StatusAdded {
			t.Errorf("second upgrade not idempotent: %s -> %s", f.Dest, f.Status)
		}
	}
}

func dropLine(t *testing.T, path, substr string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, l := range strings.Split(string(data), "\n") {
		if !strings.Contains(l, substr) {
			kept = append(kept, l)
		}
	}
	if err := os.WriteFile(path, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}

func replaceInFile(t *testing.T, path, old, new string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(data), old, new)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fileStatus(res *UpgradeResult, dest string) FileStatus {
	for _, f := range res.Files {
		if f.Dest == dest {
			return f.Status
		}
	}
	return ""
}

func TestManifestRoundTrip(t *testing.T) {
	root := t.TempDir()
	m := DefaultManifest("gitlab.com/acme/widgets", false)
	m.Blocks = append(m.Blocks, InstalledBlock{Name: "core-schema"})
	if err := SaveManifest(root, m); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}
	got, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if got.Module != m.Module || len(got.Blocks) != 1 || got.Blocks[0].Name != "core-schema" {
		t.Errorf("round-trip mismatch: %+v", got)
	}

	// FindProjectRoot locates the manifest from a nested dir.
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := FindProjectRoot(nested)
	if err != nil {
		t.Fatalf("FindProjectRoot: %v", err)
	}
	if found != root {
		t.Errorf("FindProjectRoot = %q, want %q", found, root)
	}
}
