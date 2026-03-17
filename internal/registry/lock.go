package registry

// Lockfile is ix.lock — tool-managed, committed. It records the resolved state
// that makes upgrades deterministic: per-block version, the runtime module dep,
// and the SHA-256 of the last pristine render of every managed file (the
// baseline for 3-way merge, docs/DESIGN.md §7).
type Lockfile struct {
	Version         int                  `yaml:"version"`
	CLIVersion      string               `yaml:"cli_version"`
	RegistryVersion string               `yaml:"registry_version"`
	Blocks          map[string]LockBlock `yaml:"blocks"`
}

type LockBlock struct {
	Version string              `yaml:"version"`
	Runtime *LockRuntime        `yaml:"runtime,omitempty"`
	Files   map[string]LockFile `yaml:"files"`
}

type LockRuntime struct {
	Module  string `yaml:"module"`
	Version string `yaml:"version"`
}

type LockFile struct {
	// Hash is the SHA-256 ("sha256:…") of the last pristine render — the BASE
	// for 3-way merge.
	Hash string `yaml:"hash"`
	// Managed false ⇒ rendered once on add, never touched by upgrade.
	Managed bool `yaml:"managed"`
}
