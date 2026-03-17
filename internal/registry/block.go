package registry

// Category classifies a block by what it contributes (docs/DESIGN.md §6).
type Category string

const (
	CategoryRuntimeGlue Category = "runtime-glue" // vendored wiring around a runtime/ package
	CategoryCodegen     Category = "codegen"      // the schema→sqlc→gqlgen pipeline pieces
	CategoryInfra       Category = "infra"        // docker, compose, helm, CI
	CategoryFrontend    Category = "frontend"     // SvelteKit / shadcn-svelte / Houdini
	CategoryGenerator   Category = "generator"    // scaffolds new code on demand (e.g. entity)
)

// Block is block.yaml — metadata for one vendorable unit.
type Block struct {
	Name     string      `yaml:"name"`
	Version  string      `yaml:"version"`
	Category Category    `yaml:"category"`
	Summary  string      `yaml:"summary"`
	Requires []string    `yaml:"requires"`          // other block names
	Runtime  *RuntimeDep `yaml:"runtime,omitempty"` // versioned go.mod dependency
	Vars     []Var       `yaml:"vars"`              // template variables
	Files    []FileSpec  `yaml:"files"`             // templated files to materialize
	Patches  []Patch     `yaml:"patches"`           // structured edits into other blocks' files
	Hooks    Hooks       `yaml:"hooks"`
}

type RuntimeDep struct {
	Module   string   `yaml:"module"`
	Version  string   `yaml:"version"`
	Packages []string `yaml:"packages"`
}

// Var is a template variable, resolved from a manifest field (From) or prompted.
type Var struct {
	Name string `yaml:"name"`
	From string `yaml:"from,omitempty"` // manifest field, e.g. "module"
}

// FileSpec is one rendered file. Src is relative to the block's templates/ dir;
// Dest is a Go-template'd, project-relative path.
type FileSpec struct {
	Src     string `yaml:"src"`
	Dest    string `yaml:"dest"`
	Managed *bool  `yaml:"managed,omitempty"` // default true; false ⇒ render once
	Once    bool   `yaml:"once,omitempty"`    // skip if dest already exists
	When    string `yaml:"when,omitempty"`    // render only if this bool var is true
	Raw     bool   `yaml:"raw,omitempty"`     // copy verbatim (no templating) — e.g. Helm chart bodies
}

// Patch injects Insert into an existing file at a named Anchor comment, and
// ensures Imports are present in the file's import group. Applied idempotently
// (insert skipped if already present; imports deduped).
type Patch struct {
	File    string   `yaml:"file"`
	Anchor  string   `yaml:"anchor"`
	Insert  string   `yaml:"insert"`
	Imports []string `yaml:"imports,omitempty"` // Go import paths to add (templated)
}

type Hooks struct {
	PostAdd     []string `yaml:"post_add,omitempty"`
	PostUpgrade []string `yaml:"post_upgrade,omitempty"`
}

// Index is registry.json — the index of all available blocks.
type Index struct {
	Version string       `json:"version"`
	Blocks  []IndexEntry `json:"blocks"`
}

type IndexEntry struct {
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Category Category `json:"category"`
	Summary  string   `json:"summary"`
}
