package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"1.0.0", "v1.0.0", 0},
		{"v0.1.0", "v0.2.0", -1},
		{"v0.10.0", "v0.9.9", 1},
		{"v1.0.0", "v1.0.1", -1},
		{"v2.0.0", "v1.9.9", 1},
		{"v1.0", "v1.0.0", 0},
		{"v1.0.0-rc1", "v1.0.0", -1},
		{"v1.0.0-rc1", "v1.0.0-rc2", -1},
		{"v1.0.0+build5", "v1.0.0", 0},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestGoModRequire(t *testing.T) {
	root := t.TempDir()
	gomod := `module gitlab.com/acme/widgets

go 1.25.0

require (
	github.com/appsome/ix/runtime v0.3.0 // indirect
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/single/line v1.2.3

replace github.com/replaced/mod => ../local
`
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}

	v, replaced, err := GoModRequire(root, "github.com/appsome/ix/runtime")
	if err != nil {
		t.Fatalf("GoModRequire: %v", err)
	}
	if v != "v0.3.0" || replaced {
		t.Errorf("runtime: got (%q, %v), want (v0.3.0, false)", v, replaced)
	}

	if v, _, _ := GoModRequire(root, "github.com/single/line"); v != "v1.2.3" {
		t.Errorf("single-line require: got %q, want v1.2.3", v)
	}

	if v, _, _ := GoModRequire(root, "github.com/not/there"); v != "" {
		t.Errorf("absent module: got %q, want empty", v)
	}

	if _, replaced, _ := GoModRequire(root, "github.com/replaced/mod"); !replaced {
		t.Error("replaced module not detected")
	}

	if _, _, err := GoModRequire(t.TempDir(), "x"); err == nil {
		t.Error("missing go.mod: expected an error")
	}
}

// TestCheckDrift installs a real block, then exercises each drift status by
// mutating the working tree.
func TestCheckDrift(t *testing.T) {
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

	byDest := func() map[string]DriftStatus {
		out := map[string]DriftStatus{}
		for _, d := range CheckDrift(root, lock) {
			if d.Block != "core-schema" {
				t.Errorf("unexpected block %q in drift report", d.Block)
			}
			out[d.Dest] = d.Status
		}
		return out
	}

	// Fresh install: everything pristine.
	for dest, status := range byDest() {
		if status != DriftClean {
			t.Errorf("fresh install: %s = %s, want clean", dest, status)
		}
	}

	// Local edit → modified.
	if err := os.WriteFile(filepath.Join(root, "sqlc.yaml"), []byte("# edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Deleted file → missing.
	if err := os.Remove(filepath.Join(root, "atlas.hcl")); err != nil {
		t.Fatal(err)
	}
	// Lost baseline → no-baseline.
	if err := os.Remove(filepath.Join(root, baselineDir, "scripts/generate.sh")); err != nil {
		t.Fatal(err)
	}

	got := byDest()
	want := map[string]DriftStatus{
		"sqlc.yaml":           DriftModified,
		"atlas.hcl":           DriftMissing,
		"scripts/generate.sh": DriftNoBaseline,
	}
	for dest, status := range want {
		if got[dest] != status {
			t.Errorf("%s = %s, want %s", dest, got[dest], status)
		}
	}

	// Unmanaged files must not appear in the report.
	lock.Blocks["fake"] = LockBlock{Version: "0.0.1", Files: map[string]LockFile{
		"README.md": {Hash: "sha256:0", Managed: false},
	}}
	for _, d := range CheckDrift(root, lock) {
		if d.Block == "fake" {
			t.Errorf("unmanaged file reported: %+v", d)
		}
	}
}
