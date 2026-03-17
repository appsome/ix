package registry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"text/template"
)

// baselineDir is the project-relative directory holding pristine renders of
// every managed file — the BASE for the phase-5 three-way merge.
const baselineDir = ".ix/baseline"

// InstallOptions tunes a block installation.
type InstallOptions struct {
	// Params are command-supplied template variables (e.g. entity --name),
	// merged over the manifest-derived vars.
	Params map[string]any
	// RunHooks executes the block's post_add hooks when true.
	RunHooks bool
	// Logf receives human-facing progress lines. Required.
	Logf func(format string, args ...any)
}

// InstallResult reports what an installation changed.
type InstallResult struct {
	Written []string
	Skipped []string
	Patched []string
}

// Install materializes a block into the project rooted at root, updating m and
// lock in place (the caller persists them). It renders the block's files,
// applies its anchor patches, records pristine baselines + hashes, appends the
// block to the manifest, and optionally runs post_add hooks.
func Install(root string, m *Manifest, lock *Lockfile, b *Block, opts InstallOptions) (*InstallResult, error) {
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}

	// Dependency check — warn rather than fail, so a user can add in any order
	// and resolve gaps themselves.
	for _, req := range b.Requires {
		if !manifestHasBlock(m, req) {
			opts.Logf("  ! %s requires %q, which is not installed yet", b.Name, req)
		}
	}

	vars, err := resolveVars(b, m, opts.Params)
	if err != nil {
		return nil, err
	}

	res := &InstallResult{}
	// Seed from any existing lock entry so a re-add (e.g. to pick up a new
	// file the block gained) preserves the hashes of files left in place,
	// rather than narrowing the lock to only what this run wrote.
	lockFiles := map[string]LockFile{}
	if prev, ok := lock.Blocks[b.Name]; ok {
		maps.Copy(lockFiles, prev.Files)
	}

	for _, f := range b.Files {
		if !fileApplies(f, vars) {
			continue
		}
		dest, err := renderString("dest", f.Dest, vars)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", b.Name, err)
		}
		absDest := filepath.Join(root, dest)

		if _, statErr := os.Stat(absDest); statErr == nil {
			// Idempotent add: never clobber an existing file. once + managed:false
			// files are render-once by design; managed files are left for
			// `ix upgrade` to merge.
			res.Skipped = append(res.Skipped, dest)
			opts.Logf("  = %s (exists, skipped)", dest)
			continue
		}

		rendered, err := renderFile(b.Name, f, vars)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", b.Name, err)
		}

		if err := writeFile(absDest, rendered); err != nil {
			return nil, err
		}
		res.Written = append(res.Written, dest)
		opts.Logf("  + %s", dest)

		managed := f.Managed == nil || *f.Managed
		if managed {
			if err := writeFile(filepath.Join(root, baselineDir, dest), rendered); err != nil {
				return nil, err
			}
		}
		lockFiles[dest] = LockFile{Hash: hashBytes(rendered), Managed: managed}
	}

	// Patches inject into files owned by other blocks at named anchors.
	for _, p := range b.Patches {
		applied, err := applyPatch(root, p, vars)
		if err != nil {
			opts.Logf("  ! patch %s: %v", p.File, err)
			continue
		}
		if applied {
			res.Patched = append(res.Patched, p.File)
			opts.Logf("  ~ patched %s at %q", p.File, p.Anchor)
		} else {
			opts.Logf("  = %s already patched, skipped", p.File)
		}
	}

	// Update manifest + lock.
	if !manifestHasBlock(m, b.Name) {
		m.Blocks = append(m.Blocks, InstalledBlock{Name: b.Name})
	}
	lb := LockBlock{Version: b.Version, Files: lockFiles}
	if b.Runtime != nil {
		lb.Runtime = &LockRuntime{Module: b.Runtime.Module, Version: b.Runtime.Version}
	}
	lock.Blocks[b.Name] = lb

	// Hooks may reference template vars (e.g. the entity name), so render them.
	hooks := make([]string, 0, len(b.Hooks.PostAdd))
	for _, h := range b.Hooks.PostAdd {
		rendered, err := renderString("hook", h, vars)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", b.Name, err)
		}
		hooks = append(hooks, rendered)
	}
	if opts.RunHooks {
		runHooks(root, hooks, opts.Logf)
	} else if len(hooks) > 0 {
		opts.Logf("  next steps (post_add hooks, not run):")
		for _, h := range hooks {
			opts.Logf("    $ %s", h)
		}
	}

	return res, nil
}

// resolveVars builds the template variable map from the block's declared vars
// (resolved against the manifest) plus always-available built-ins and any
// command params.
func resolveVars(b *Block, m *Manifest, params map[string]any) (map[string]any, error) {
	vars := map[string]any{
		"Module":    m.Module,
		"App":       path.Base(m.Module), // last module segment: image/chart/service name
		"PostGIS":   m.Database.PostGIS,
		"Timestamp": timestamp(),
	}
	for _, v := range b.Vars {
		if v.From == "" {
			continue
		}
		val, err := manifestField(m, v.From)
		if err != nil {
			return nil, fmt.Errorf("%s: var %q: %w", b.Name, v.Name, err)
		}
		vars[v.Name] = val
	}
	maps.Copy(vars, params)
	return vars, nil
}

func manifestField(m *Manifest, from string) (any, error) {
	switch from {
	case "module":
		return m.Module, nil
	case "database.engine":
		return m.Database.Engine, nil
	case "database.postgis":
		return m.Database.PostGIS, nil
	default:
		return nil, fmt.Errorf("unknown manifest field %q", from)
	}
}

// fileApplies reports whether a conditional file (When references a bool var)
// should be rendered. An empty When always applies.
func fileApplies(f FileSpec, vars map[string]any) bool {
	if f.When == "" {
		return true
	}
	v, ok := vars[f.When]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// renderFile reads a block's source template and returns its content for the
// destination — rendered through text/template, or copied verbatim when the
// FileSpec is Raw (e.g. Helm chart bodies that are themselves Go templates).
func renderFile(block string, f FileSpec, vars map[string]any) ([]byte, error) {
	raw, err := BlockTemplate(block, f.Src)
	if err != nil {
		return nil, err
	}
	if f.Raw {
		return raw, nil
	}
	return renderBytes(f.Src, raw, vars)
}

// RenderTemplate renders a Go text/template body with vars. Exported for the
// CLI's base-scaffold renderer; block files go through Install.
func RenderTemplate(name string, body []byte, vars map[string]any) ([]byte, error) {
	return renderBytes(name, body, vars)
}

func renderBytes(name string, body []byte, vars map[string]any) ([]byte, error) {
	t, err := template.New(name).Option("missingkey=error").Parse(string(body))
	if err != nil {
		return nil, fmt.Errorf("parse template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, vars); err != nil {
		return nil, fmt.Errorf("render template %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

func renderString(name, body string, vars map[string]any) (string, error) {
	out, err := renderBytes(name, []byte(body), vars)
	return string(out), err
}

func writeFile(absPath string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(absPath), err)
	}
	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", absPath, err)
	}
	return nil
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func manifestHasBlock(m *Manifest, name string) bool {
	for _, b := range m.Blocks {
		if b.Name == name {
			return true
		}
	}
	return false
}

// applyPatch ensures the patch's imports are present and inserts its text after
// the anchor line (re-indented to match). Both steps are idempotent; it returns
// true if it changed anything.
func applyPatch(root string, p Patch, vars map[string]any) (bool, error) {
	abs := filepath.Join(root, p.File)
	content, err := os.ReadFile(abs)
	if err != nil {
		return false, fmt.Errorf("read target: %w", err)
	}
	insert, err := renderString("patch", p.Insert, vars)
	if err != nil {
		return false, err
	}
	insert = strings.TrimRight(insert, "\n")

	text := string(content)
	changed := false

	// 1. Ensure imports.
	imports := make([]string, 0, len(p.Imports))
	for _, imp := range p.Imports {
		rendered, err := renderString("import", imp, vars)
		if err != nil {
			return false, err
		}
		imports = append(imports, rendered)
	}
	if newText, added := addImports(text, imports); added {
		text = newText
		changed = true
	}

	// 2. Insert the body at the anchor, unless already present.
	if !strings.Contains(text, strings.TrimSpace(firstLine(insert))) {
		inserted, err := insertAtAnchor(text, p.Anchor, insert)
		if err != nil {
			return false, err
		}
		text = inserted
		changed = true
	}

	if !changed {
		return false, nil
	}
	return true, writeFile(abs, []byte(text))
}

// insertAtAnchor inserts block after the line whose trimmed content equals
// anchor (a standalone marker line — so a prose mention of the anchor token in
// a doc comment is never treated as the insertion point), re-indented to match.
func insertAtAnchor(text, anchor, block string) (string, error) {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != anchor {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		reindented := reindent(block, indent)
		out := append([]string{}, lines[:i+1]...)
		out = append(out, strings.Split(reindented, "\n")...)
		out = append(out, lines[i+1:]...)
		return strings.Join(out, "\n"), nil
	}
	return "", fmt.Errorf("anchor %q not found", anchor)
}

// addImports inserts any missing import paths into the file's `import ( … )`
// group. Returns the new text and whether anything was added. Requires a
// grouped import block; single-line imports are left untouched.
func addImports(text string, imports []string) (string, bool) {
	if len(imports) == 0 {
		return text, false
	}
	lines := strings.Split(text, "\n")
	open := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "import (" {
			open = i
			break
		}
	}
	if open == -1 {
		return text, false
	}
	closeIdx := -1
	for i := open + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == ")" {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		return text, false
	}

	existing := strings.Join(lines[open:closeIdx+1], "\n")

	// New imports go into the last import group (the project-local one, after
	// the final blank line in the block) in sorted position, so the result
	// stays gofmt-clean even when several blocks each add a local import — e.g.
	// internal/auth must land before internal/authz. groupStart is the first
	// line of that group.
	groupStart := open + 1
	for i := open + 1; i < closeIdx; i++ {
		if strings.TrimSpace(lines[i]) == "" {
			groupStart = i + 1
		}
	}

	added := false
	for _, imp := range imports {
		if strings.Contains(existing, `"`+imp+`"`) {
			continue
		}
		// Sorted insertion point within the last group.
		pos := closeIdx
		for i := groupStart; i < closeIdx; i++ {
			if importPath(lines[i]) > imp {
				pos = i
				break
			}
		}
		entry := "\t\"" + imp + "\""
		lines = append(lines[:pos], append([]string{entry}, lines[pos:]...)...)
		closeIdx++ // the block grew by one line
		added = true
	}
	if !added {
		return text, false
	}
	return strings.Join(lines, "\n"), true
}

// importPath returns the quoted path from an import line for lexical
// comparison. Lines with no quoted path sort last within their group.
func importPath(line string) string {
	_, rest, ok := strings.Cut(line, `"`)
	if !ok {
		return "￿"
	}
	path, _, ok := strings.Cut(rest, `"`)
	if !ok {
		return "￿"
	}
	return path
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// reindent strips the common leading whitespace from block and re-applies
// indent to each non-empty line.
func reindent(block, indent string) string {
	lines := strings.Split(block, "\n")
	common := commonIndent(lines)
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = indent + strings.TrimPrefix(l, common)
	}
	return strings.Join(lines, "\n")
}

func commonIndent(lines []string) string {
	common := ""
	first := true
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		lead := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		if first {
			common = lead
			first = false
			continue
		}
		common = commonPrefix(common, lead)
	}
	return common
}

func commonPrefix(a, b string) string {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return a[:i]
}

func runHooks(root string, hooks []string, logf func(string, ...any)) {
	for _, h := range hooks {
		_ = runHookCmd(root, h, logf)
	}
}

// runHookCmd runs a single shell command in root, streaming its combined output
// through logf. Returns the command error (already logged).
func runHookCmd(root, command string, logf func(string, ...any)) error {
	logf("  $ %s", command)
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		logf("%s", strings.TrimRight(string(out), "\n"))
	}
	if err != nil {
		logf("  ! command failed: %v", err)
	}
	return err
}
