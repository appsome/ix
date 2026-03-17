// Package registry defines the on-disk formats that the ix CLI reads and
// writes: the project manifest (ix.yaml), the lockfile (ix.lock), the block
// index (registry.json), and per-block metadata (block.yaml). See
// docs/DESIGN.md §5 and §6.
//
// This file declares the types only — load/save/validate logic lands in
// phase 3. They are the source of truth for the formats documented in DESIGN.
package registry

// Manifest is ix.yaml — the human-edited project configuration.
type Manifest struct {
	Version  int              `yaml:"version"`
	Module   string           `yaml:"module"`
	Database DatabaseConfig   `yaml:"database"`
	Paths    Paths            `yaml:"paths"`
	Frontend FrontendConfig   `yaml:"frontend"`
	Blocks   []InstalledBlock `yaml:"blocks"`
	Runtime  string           `yaml:"runtime"`
}

type DatabaseConfig struct {
	Engine  string `yaml:"engine"`  // postgresql
	PostGIS bool   `yaml:"postgis"` // toggles the PostGIS extension + geom override
}

type Paths struct {
	Schema        string `yaml:"schema"`
	Queries       string `yaml:"queries"`
	Migrations    string `yaml:"migrations"`
	GraphQLSchema string `yaml:"graphql_schema"`
	Web           string `yaml:"web"`
}

type FrontendConfig struct {
	Framework string `yaml:"framework"` // sveltekit
	UI        string `yaml:"ui"`        // shadcn-svelte
	GraphQL   string `yaml:"graphql"`   // houdini
}

// InstalledBlock is one entry in Manifest.Blocks. Resolved version + file
// hashes live in the Lockfile, not here.
type InstalledBlock struct {
	Name string `yaml:"name"`
}
