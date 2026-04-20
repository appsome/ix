package registry

import "strings"

// This file implements the line-based three-way merge that powers `ix upgrade`
// (docs/DESIGN.md §7). Given BASE (the last pristine render, from .ix/baseline),
// LOCAL (the user's current file) and UPSTREAM (the new pristine render), it
// produces a merged file, marking conflicts with the usual git-style markers.
//
// The algorithm: diff BASE→LOCAL and BASE→UPSTREAM into hunks expressed over
// BASE line coordinates, then walk BASE emitting stable regions verbatim and,
// where one or both sides changed, taking the lone change or — when both sides
// changed the same span differently — a conflict block.

const (
	conflictStart = "<<<<<<< local (your changes)"
	conflictMid   = "======="
	conflictEnd   = ">>>>>>> upstream"
)

// hunk records that base[BaseStart:BaseEnd] is replaced by Lines on one side.
type hunk struct {
	BaseStart int
	BaseEnd   int
	Lines     []string
}

// lcsPairs returns aligned indices (ai, bi) of a longest common subsequence of
// equal lines between a and b, in increasing order.
func lcsPairs(a, b []string) [][2]int {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var pairs [][2]int
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			pairs = append(pairs, [2]int{i, j})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}
	return pairs
}

// diffHunks expresses how other differs from base, as replacements over base
// coordinates. Hunks are sorted by BaseStart, non-overlapping, BaseStart<=BaseEnd
// (BaseStart==BaseEnd is a pure insertion before that base line).
func diffHunks(base, other []string) []hunk {
	pairs := lcsPairs(base, other)
	var hunks []hunk
	pb, po := 0, 0
	emit := func(bEnd, oEnd int) {
		if bEnd > pb || oEnd > po {
			hunks = append(hunks, hunk{BaseStart: pb, BaseEnd: bEnd, Lines: append([]string{}, other[po:oEnd]...)})
		}
	}
	for _, pr := range pairs {
		emit(pr[0], pr[1])
		pb, po = pr[0]+1, pr[1]+1
	}
	emit(len(base), len(other))
	return hunks
}

// merge3 performs the three-way merge. It returns the merged lines and whether
// any conflict markers were emitted.
func merge3(base, local, upstream []string) ([]string, bool) {
	ha := diffHunks(base, local)
	hb := diffHunks(base, upstream)

	var out []string
	conflict := false
	n := len(base)
	i, ai, bi := 0, 0, 0

	for i < n || ai < len(ha) || bi < len(hb) {
		nextA := n + 1
		if ai < len(ha) {
			nextA = ha[ai].BaseStart
		}
		nextB := n + 1
		if bi < len(hb) {
			nextB = hb[bi].BaseStart
		}
		next := min(nextA, nextB)

		// Stable region: base lines before the next change, common to all.
		if i < next && i < n {
			end := min(next, n)
			out = append(out, base[i:end]...)
			i = end
			continue
		}

		// A change region begins at i. First consume the trigger hunks that
		// start exactly at i (including zero-width insertions, which abut but
		// do not overlap their neighbours). Then expand the region only over
		// hunks that *strictly* overlap it — an insertion at the region's end
		// boundary belongs to the next region, not this one, so independent
		// edits don't collapse into a false conflict.
		regionEnd := i
		aj, bj := ai, bi
		changedA, changedB := false, false
		for aj < len(ha) && ha[aj].BaseStart == i {
			if ha[aj].BaseEnd > regionEnd {
				regionEnd = ha[aj].BaseEnd
			}
			changedA = true
			aj++
		}
		for bj < len(hb) && hb[bj].BaseStart == i {
			if hb[bj].BaseEnd > regionEnd {
				regionEnd = hb[bj].BaseEnd
			}
			changedB = true
			bj++
		}
		for {
			extended := false
			for aj < len(ha) && ha[aj].BaseStart < regionEnd {
				if ha[aj].BaseEnd > regionEnd {
					regionEnd = ha[aj].BaseEnd
					extended = true
				}
				changedA = true
				aj++
			}
			for bj < len(hb) && hb[bj].BaseStart < regionEnd {
				if hb[bj].BaseEnd > regionEnd {
					regionEnd = hb[bj].BaseEnd
					extended = true
				}
				changedB = true
				bj++
			}
			if !extended {
				break
			}
		}

		localLines := applyHunks(base, ha[ai:aj], i, regionEnd)
		upstreamLines := applyHunks(base, hb[bi:bj], i, regionEnd)

		switch {
		case changedA && changedB:
			if equalLines(localLines, upstreamLines) {
				out = append(out, localLines...)
			} else {
				out = append(out, conflictStart)
				out = append(out, localLines...)
				out = append(out, conflictMid)
				out = append(out, upstreamLines...)
				out = append(out, conflictEnd)
				conflict = true
			}
		case changedA:
			out = append(out, localLines...)
		case changedB:
			out = append(out, upstreamLines...)
		default:
			out = append(out, base[i:regionEnd]...)
		}

		ai, bi = aj, bj
		i = regionEnd
	}

	return out, conflict
}

// applyHunks reconstructs one side's content for base[start:end], given that
// side's hunks within the span (sorted, non-overlapping, inside [start,end]).
func applyHunks(base []string, hs []hunk, start, end int) []string {
	var out []string
	pos := start
	for _, h := range hs {
		if h.BaseStart > pos {
			out = append(out, base[pos:h.BaseStart]...)
		}
		out = append(out, h.Lines...)
		pos = h.BaseEnd
	}
	if pos < end {
		out = append(out, base[pos:end]...)
	}
	return out
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// splitLines splits file content into lines, dropping a single trailing newline
// so a final "\n" doesn't appear as a spurious empty line in the diff/merge.
func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "\n")
}

// joinLines is the inverse of splitLines, restoring the trailing newline.
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// unifiedDiff renders a compact unified diff (3 lines of context) of a→b.
func unifiedDiff(path string, a, b []string) string {
	pairs := lcsPairs(a, b)
	// Build per-line ops by walking the LCS.
	type op struct {
		kind byte // ' ', '-', '+'
		text string
	}
	var ops []op
	ai, bi, pi := 0, 0, 0
	for pi <= len(pairs) {
		var na, nb int
		if pi < len(pairs) {
			na, nb = pairs[pi][0], pairs[pi][1]
		} else {
			na, nb = len(a), len(b)
		}
		for ai < na {
			ops = append(ops, op{'-', a[ai]})
			ai++
		}
		for bi < nb {
			ops = append(ops, op{'+', b[bi]})
			bi++
		}
		if pi < len(pairs) {
			ops = append(ops, op{' ', a[na]})
			ai, bi = na+1, nb+1
		}
		pi++
	}

	// Emit only regions with changes, with up to 3 context lines around them.
	const ctx = 3
	keep := make([]bool, len(ops))
	for i, o := range ops {
		if o.kind != ' ' {
			for j := max(0, i-ctx); j <= min(len(ops)-1, i+ctx); j++ {
				keep[j] = true
			}
		}
	}
	var sb strings.Builder
	sb.WriteString("--- " + path + " (current)\n")
	sb.WriteString("+++ " + path + " (after upgrade)\n")
	prev := false
	any := false
	for i, o := range ops {
		if !keep[i] {
			prev = false
			continue
		}
		if !prev && any {
			sb.WriteString("@@\n")
		}
		sb.WriteByte(o.kind)
		sb.WriteString(o.text)
		sb.WriteByte('\n')
		prev = true
		any = true
	}
	if !any {
		return ""
	}
	return sb.String()
}
