package registry

import (
	"fmt"
	"os"
	"path/filepath"
)

// FileStatus is the outcome of upgrading a single file.
type FileStatus string

const (
	StatusAdded     FileStatus = "added"     // new file introduced by the new version
	StatusMerged    FileStatus = "merged"    // upstream change merged cleanly into local edits
	StatusConflict  FileStatus = "conflict"  // merged with conflict markers
	StatusUnchanged FileStatus = "unchanged" // upstream identical to baseline — nothing to do
	StatusSkipped   FileStatus = "skipped"   // render-once / unmanaged / no baseline
)

// FileUpgrade reports what happened (or would happen) to one file.
type FileUpgrade struct {
	Dest   string
	Status FileStatus
	Note   string // reason for skip, etc.
	Diff   string // unified diff of current → result (populated on dry run / changes)
}

// UpgradeResult summarizes a block upgrade.
type UpgradeResult struct {
	Block         string
	FromVersion   string
	ToVersion     string
	Files         []FileUpgrade
	RuntimeFrom   string
	RuntimeTo     string
	RuntimeBumped bool
}

// HasConflicts reports whether any file ended in a conflict.
func (r *UpgradeResult) HasConflicts() bool {
	for _, f := range r.Files {
		if f.Status == StatusConflict {
			return true
		}
	}
	return false
}

// UpgradeOptions tunes an upgrade.
type UpgradeOptions struct {
	DryRun   bool // compute diffs, write nothing (ix diff)
	RunHooks bool // run post_upgrade hooks + `go get` for runtime bumps
	Logf     func(format string, args ...any)
}

// Upgrade re-renders a block at the CLI's embedded (new) version and three-way
// merges each managed file against the user's working copy, using the committed
// .ix/baseline pristine copy as the merge base (docs/DESIGN.md §7). On a dry run
// it writes nothing and fills in diffs. Generator blocks are not upgradeable
// (their files are per-instance, render-once) and are reported as skipped.
func Upgrade(root string, m *Manifest, lock *Lockfile, b *Block, opts UpgradeOptions) (*UpgradeResult, error) {
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	prev := lock.Blocks[b.Name]
	res := &UpgradeResult{Block: b.Name, FromVersion: prev.Version, ToVersion: b.Version}

	if b.Category == CategoryGenerator {
		res.Files = append(res.Files, FileUpgrade{Status: StatusSkipped, Note: "generator blocks are not upgradeable in place"})
		return res, nil
	}

	vars, err := resolveVars(b, m, nil)
	if err != nil {
		return nil, err
	}

	newLockFiles := map[string]LockFile{}
	// Preserve lock entries for files we don't touch (managed:false, once).
	for dest, lf := range prev.Files {
		newLockFiles[dest] = lf
	}

	for _, f := range b.Files {
		if !fileApplies(f, vars) {
			continue
		}
		managed := f.Managed == nil || *f.Managed
		dest, err := renderString("dest", f.Dest, vars)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", b.Name, err)
		}
		if f.Once || !managed {
			continue // render-once / unmanaged: never touched by upgrade
		}

		oursBytes, err := renderFile(b.Name, f, vars)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", b.Name, err)
		}

		absDest := filepath.Join(root, dest)
		absBase := filepath.Join(root, baselineDir, dest)

		theirsBytes, statErr := os.ReadFile(absDest)
		if statErr != nil {
			// New file in this version: just add it.
			fu := FileUpgrade{Dest: dest, Status: StatusAdded, Diff: unifiedDiff(dest, nil, splitLines(string(oursBytes)))}
			if !opts.DryRun {
				if err := writeFile(absDest, oursBytes); err != nil {
					return nil, err
				}
				if err := writeFile(absBase, oursBytes); err != nil {
					return nil, err
				}
			}
			newLockFiles[dest] = LockFile{Hash: hashBytes(oursBytes), Managed: true}
			res.Files = append(res.Files, fu)
			continue
		}

		baseBytes, baseErr := os.ReadFile(absBase)
		if baseErr != nil {
			res.Files = append(res.Files, FileUpgrade{Dest: dest, Status: StatusSkipped, Note: "no .ix/baseline copy — cannot merge safely; resolve manually"})
			continue
		}

		// No upstream change ⇒ nothing to do, regardless of local edits.
		if string(oursBytes) == string(baseBytes) {
			res.Files = append(res.Files, FileUpgrade{Dest: dest, Status: StatusUnchanged})
			continue
		}

		merged, conflict := merge3(splitLines(string(baseBytes)), splitLines(string(theirsBytes)), splitLines(string(oursBytes)))
		mergedStr := joinLines(merged)

		fu := FileUpgrade{Dest: dest, Diff: unifiedDiff(dest, splitLines(string(theirsBytes)), merged)}
		if conflict {
			fu.Status = StatusConflict
		} else {
			fu.Status = StatusMerged
		}
		if !opts.DryRun {
			if err := writeFile(absDest, []byte(mergedStr)); err != nil {
				return nil, err
			}
			// Baseline tracks the pristine upstream render, not the merged
			// result, so the next upgrade's BASE is correct.
			if err := writeFile(absBase, oursBytes); err != nil {
				return nil, err
			}
			newLockFiles[dest] = LockFile{Hash: hashBytes(oursBytes), Managed: true}
		}
		res.Files = append(res.Files, fu)
	}

	// Runtime dependency bump.
	if b.Runtime != nil {
		res.RuntimeTo = b.Runtime.Version
		if prev.Runtime != nil {
			res.RuntimeFrom = prev.Runtime.Version
		}
		if res.RuntimeFrom != res.RuntimeTo {
			res.RuntimeBumped = true
			if !opts.DryRun && opts.RunHooks {
				_ = runGoGet(root, b.Runtime.Module, b.Runtime.Version, opts.Logf)
			}
		}
	}

	if !opts.DryRun {
		// Re-apply patches (idempotent) in case the new version added any.
		for _, p := range b.Patches {
			if _, err := applyPatch(root, p, vars); err != nil {
				opts.Logf("  ! patch %s: %v", p.File, err)
			}
		}
		nb := LockBlock{Version: b.Version, Files: newLockFiles}
		if b.Runtime != nil {
			nb.Runtime = &LockRuntime{Module: b.Runtime.Module, Version: b.Runtime.Version}
		}
		lock.Blocks[b.Name] = nb
		if opts.RunHooks {
			renderAndRunHooks(root, b.Hooks.PostUpgrade, vars, opts.Logf)
		}
	}

	return res, nil
}

func runGoGet(root, module, version string, logf func(string, ...any)) error {
	target := module
	if version != "" {
		target = module + "@" + version
	}
	logf("  $ go get %s", target)
	return runHookCmd(root, "go get "+target, logf)
}

func renderAndRunHooks(root string, hooks []string, vars map[string]any, logf func(string, ...any)) {
	for _, h := range hooks {
		rendered, err := renderString("hook", h, vars)
		if err != nil {
			logf("  ! hook render: %v", err)
			continue
		}
		_ = runHookCmd(root, rendered, logf)
	}
}
