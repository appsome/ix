package registry

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"

	ix "github.com/appsome/ix"
	"gopkg.in/yaml.v3"
)

// registryRoot is the embedded path prefix for registry assets.
const registryRoot = "registry"

// LoadIndex parses the embedded registry.json.
func LoadIndex() (*Index, error) {
	data, err := ix.RegistryFS.ReadFile(path.Join(registryRoot, "registry.json"))
	if err != nil {
		return nil, fmt.Errorf("registry: read index: %w", err)
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("registry: parse index: %w", err)
	}
	return &idx, nil
}

// LoadBlock parses the block.yaml for the named block.
func LoadBlock(name string) (*Block, error) {
	data, err := ix.RegistryFS.ReadFile(path.Join(registryRoot, "blocks", name, "block.yaml"))
	if err != nil {
		return nil, fmt.Errorf("registry: block %q not found (no block.yaml): %w", name, err)
	}
	var b Block
	if err := yaml.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("registry: parse block %q: %w", name, err)
	}
	if b.Name == "" {
		b.Name = name
	}
	return &b, nil
}

// BlockTemplate reads a raw template file (src relative to the block's
// templates/ dir) from the embedded registry.
func BlockTemplate(block, src string) ([]byte, error) {
	p := path.Join(registryRoot, "blocks", block, "templates", src)
	data, err := ix.RegistryFS.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("registry: template %s/%s: %w", block, src, err)
	}
	return data, nil
}

// ProjectScaffold returns the embedded base project scaffold filesystem rooted
// at templates/project, plus a walk of its files (paths relative to that root).
func ProjectScaffold() (fs.FS, []string, error) {
	root := "templates/project"
	sub, err := fs.Sub(ix.TemplatesFS, root)
	if err != nil {
		return nil, nil, fmt.Errorf("registry: scaffold root: %w", err)
	}
	var files []string
	err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("registry: walk scaffold: %w", err)
	}
	return sub, files, nil
}
