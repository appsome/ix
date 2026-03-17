package registry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ManifestFile and LockFileName are the project-root filenames ix manages.
const (
	ManifestFile = "ix.yaml"
	LockFileName = "ix.lock"
)

// ErrNoManifest is returned when no ix.yaml is found walking up from a dir.
var ErrNoManifest = errors.New("registry: no ix.yaml found (run `ix init` first)")

// FindProjectRoot walks up from start looking for an ix.yaml and returns the
// directory containing it.
func FindProjectRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNoManifest
		}
		dir = parent
	}
}

// LoadManifest reads and parses ix.yaml from root.
func LoadManifest(root string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(root, ManifestFile))
	if err != nil {
		return nil, fmt.Errorf("registry: read manifest: %w", err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("registry: parse manifest: %w", err)
	}
	return &m, nil
}

// SaveManifest writes ix.yaml to root.
func SaveManifest(root string, m *Manifest) error {
	return writeYAML(filepath.Join(root, ManifestFile), m)
}

// LoadLock reads ix.lock from root, returning an empty lock if absent.
func LoadLock(root string) (*Lockfile, error) {
	data, err := os.ReadFile(filepath.Join(root, LockFileName))
	if errors.Is(err, os.ErrNotExist) {
		return &Lockfile{Version: 1, Blocks: map[string]LockBlock{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("registry: read lock: %w", err)
	}
	var l Lockfile
	if err := yaml.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("registry: parse lock: %w", err)
	}
	if l.Blocks == nil {
		l.Blocks = map[string]LockBlock{}
	}
	return &l, nil
}

// SaveLock writes ix.lock to root.
func SaveLock(root string, l *Lockfile) error {
	return writeYAML(filepath.Join(root, LockFileName), l)
}

func writeYAML(path string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("registry: marshal %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("registry: write %s: %w", filepath.Base(path), err)
	}
	return nil
}

// DefaultManifest returns a manifest pre-populated with conventional paths for a
// new project with the given module path.
func DefaultManifest(module string, postgis bool) *Manifest {
	return &Manifest{
		Version:  1,
		Module:   module,
		Database: DatabaseConfig{Engine: "postgresql", PostGIS: postgis},
		Paths: Paths{
			Schema:        "internal/datastore/schema.sql",
			Queries:       "internal/datastore/queries",
			Migrations:    "migrations",
			GraphQLSchema: "internal/api/schema",
			Web:           "web",
		},
		Frontend: FrontendConfig{Framework: "sveltekit", UI: "shadcn-svelte", GraphQL: "houdini"},
		Blocks:   []InstalledBlock{},
		Runtime:  "github.com/appsome/ix/runtime",
	}
}
