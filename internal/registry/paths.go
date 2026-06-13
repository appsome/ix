package registry

import (
	"path"
	"strings"
)

// destPath maps a block's rendered destination through the manifest's
// configured paths (docs/DESIGN.md §5). Blocks address the frontend by the
// conventional "web/" root; a project that relocates it via paths.web (e.g.
// `web: admin`) gets every frontend file — and patch target — under the
// configured directory instead. All other destinations are returned unchanged.
func destPath(m *Manifest, dest string) string {
	web := m.Paths.Web
	if web == "" || web == "web" {
		return dest
	}
	switch {
	case dest == "web":
		return web
	case strings.HasPrefix(dest, "web/"):
		return path.Join(web, strings.TrimPrefix(dest, "web/"))
	}
	return dest
}
