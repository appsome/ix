package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDestPath(t *testing.T) {
	def := DefaultManifest("gitlab.com/acme/widgets", false)
	moved := DefaultManifest("gitlab.com/acme/widgets", false)
	moved.Paths.Web = "admin"

	tests := []struct {
		name string
		m    *Manifest
		dest string
		want string
	}{
		{"default web untouched", def, "web/package.json", "web/package.json"},
		{"non-frontend untouched", moved, "cmd/api/main.go", "cmd/api/main.go"},
		{"web prefix remapped", moved, "web/package.json", "admin/package.json"},
		{"nested web path remapped", moved, "web/src/routes/(admin)/+layout.svelte", "admin/src/routes/(admin)/+layout.svelte"},
		{"bare web dir remapped", moved, "web", "admin"},
		{"webapp is not web", moved, "webapp/package.json", "webapp/package.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := destPath(tt.m, tt.dest); got != tt.want {
				t.Errorf("destPath(%q) = %q, want %q", tt.dest, got, tt.want)
			}
		})
	}
}

// Installing a frontend block into a project whose ix.yaml sets paths.web must
// land every file — and its lock entry and .ix/baseline copy — under the
// configured directory, not the conventional web/.
func TestInstall_RespectsWebPath(t *testing.T) {
	root := t.TempDir()
	m := DefaultManifest("gitlab.com/acme/widgets", false)
	m.Paths.Web = "admin"
	lock := &Lockfile{Version: 1, Blocks: map[string]LockBlock{}}

	b, err := LoadBlock("admin-svelte")
	if err != nil {
		t.Fatalf("LoadBlock: %v", err)
	}
	if _, err := Install(root, m, lock, b, InstallOptions{Logf: func(string, ...any) {}}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "admin/package.json")); err != nil {
		t.Errorf("admin/package.json not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "web")); !os.IsNotExist(err) {
		t.Error("web/ should not exist when paths.web = admin")
	}
	if _, err := os.Stat(filepath.Join(root, ".ix/baseline/admin/package.json")); err != nil {
		t.Errorf("baseline not written under admin/: %v", err)
	}

	files := lock.Blocks["admin-svelte"].Files
	if _, ok := files["admin/package.json"]; !ok {
		t.Error("lock entry should track admin/package.json")
	}
	for dest := range files {
		if dest == "web" || len(dest) > 4 && dest[:4] == "web/" {
			t.Errorf("lock entry still tracks %q", dest)
		}
	}
}
