// Package ix embeds the block registry and the base project scaffold so the
// compiled `ix` binary is a self-contained snapshot of every block and template
// at its build version. The embed directives live at the module root because
// embed patterns cannot reference parent directories — internal packages read
// these filesystems rather than touching the disk layout directly.
package ix

import "embed"

// RegistryFS is the embedded registry/ tree: registry.json plus blocks/<name>/
// (block.yaml + templates/). `all:` keeps dotfiles and files the default embed
// would skip.
//
//go:embed all:registry
var RegistryFS embed.FS

// TemplatesFS is the embedded templates/ tree, including the base project
// scaffold under templates/project materialized by `ix init`.
//
//go:embed all:templates
var TemplatesFS embed.FS
