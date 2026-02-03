// Module ix is the scaffolding CLI and the vendorable block registry. It is the
// "vendored, owned" half of the hybrid model (see docs/DESIGN.md §2): it
// materializes blocks into projects and keeps them upgradeable via 3-way merge.
//
// The runtime library lives in the ./runtime submodule (its own go.mod) so it
// versions and releases independently and is imported, not vendored.
module github.com/appsome/ix

go 1.25.0

require gopkg.in/yaml.v3 v3.0.1
