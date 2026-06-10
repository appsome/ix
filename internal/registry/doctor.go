package registry

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// DriftStatus classifies how one lock-tracked file relates to the working tree.
type DriftStatus string

const (
	// DriftClean — the working copy still matches the pristine render.
	DriftClean DriftStatus = "clean"
	// DriftModified — locally edited. Expected in the vendored model; upgrades
	// 3-way merge over local edits, so this is informational.
	DriftModified DriftStatus = "modified"
	// DriftMissing — tracked in ix.lock but absent from the working tree.
	DriftMissing DriftStatus = "missing"
	// DriftNoBaseline — managed file whose .ix/baseline copy is gone, so
	// upgrade has no merge BASE and will skip it.
	DriftNoBaseline DriftStatus = "no-baseline"
)

// DriftEntry is one managed file's drift verdict.
type DriftEntry struct {
	Block  string
	Dest   string
	Status DriftStatus
}

// CheckDrift compares every managed file recorded in ix.lock against the
// working tree and its .ix/baseline pristine copy. Unmanaged (render-once)
// files are the user's to edit or delete, so they are not checked. Results are
// ordered by block name, then destination path.
func CheckDrift(root string, lock *Lockfile) []DriftEntry {
	var out []DriftEntry
	for _, name := range slices.Sorted(maps.Keys(lock.Blocks)) {
		lb := lock.Blocks[name]
		for _, dest := range slices.Sorted(maps.Keys(lb.Files)) {
			lf := lb.Files[dest]
			if !lf.Managed {
				continue
			}
			e := DriftEntry{Block: name, Dest: dest}
			data, err := os.ReadFile(filepath.Join(root, dest))
			switch {
			case err != nil:
				e.Status = DriftMissing
			case !fileExists(filepath.Join(root, baselineDir, dest)):
				e.Status = DriftNoBaseline
			case hashBytes(data) == lf.Hash:
				e.Status = DriftClean
			default:
				e.Status = DriftModified
			}
			out = append(out, e)
		}
	}
	return out
}

func fileExists(abs string) bool {
	_, err := os.Stat(abs)
	return err == nil
}

// GoModRequire parses root/go.mod and returns the version module is required
// at ("" when absent), plus whether a replace directive overrides it (the
// version then reflects the require line, not what actually builds).
func GoModRequire(root, module string) (version string, replaced bool, err error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", false, fmt.Errorf("registry: read go.mod: %w", err)
	}
	inRequire, inReplace := false, false
	for line := range strings.SplitSeq(string(data), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		req, rep := inRequire, inReplace
		switch fields[0] {
		case "require", "replace":
			if len(fields) == 2 && fields[1] == "(" {
				inRequire = fields[0] == "require"
				inReplace = fields[0] == "replace"
				continue
			}
			req, rep = fields[0] == "require", fields[0] == "replace"
			fields = fields[1:]
		case ")":
			inRequire, inReplace = false, false
			continue
		}
		if len(fields) == 0 || fields[0] != module {
			continue
		}
		if rep {
			replaced = true
		}
		if req && len(fields) >= 2 {
			version = fields[1]
		}
	}
	return version, replaced, nil
}

// CompareVersions compares two semver-ish strings ("v1.2.3", "0.4.0-rc1"),
// returning -1, 0, or +1. Missing numeric parts count as zero, a pre-release
// sorts before its release, and build metadata (+…) is ignored.
func CompareVersions(a, b string) int {
	na, prea := parseVersion(a)
	nb, preb := parseVersion(b)
	for i := range na {
		if na[i] != nb[i] {
			if na[i] < nb[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case prea == preb:
		return 0
	case prea == "": // release > its pre-releases
		return 1
	case preb == "":
		return -1
	case prea < preb:
		return -1
	default:
		return 1
	}
}

func parseVersion(v string) ([3]int, string) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	var pre string
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v, pre = v[:i], v[i+1:]
	}
	var nums [3]int
	for i, part := range strings.SplitN(v, ".", 3) {
		n, _ := strconv.Atoi(part)
		nums[i] = n
	}
	return nums, pre
}
