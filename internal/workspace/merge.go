package workspace

import "strings"

// The markers of a conflict a merge leaves for the user to resolve, as git
// writes them with merge.conflictStyle diff3: what the user has, what the
// template wrote last time (the base), then what esquadro brings. With the
// base, the file alone holds the three sides an editor needs to show it.
const (
	markerOurs   = "<<<<<<< yours"
	markerBase   = "||||||| base"
	markerSplit  = "======="
	markerTheirs = ">>>>>>> esquadro"
)

// Merge3 brings the changes from base to theirs (what esquadro generates
// now) into ours (the user's file, changed from base), line by line, as
// git merge-file does: a part only one side changed takes that side's
// lines; a part both changed alike takes them once; a part both changed
// differently is a conflict, kept with both sides between markers. ok is
// false when the files are too large to compare.
func Merge3(base, ours, theirs []byte) (merged []byte, conflicts int, ok bool) {
	b, o, t := splitLines(string(base)), splitLines(string(ours)), splitLines(string(theirs))
	if len(b)*len(o) > maxDiffCells || len(b)*len(t) > maxDiffCells {
		return nil, 0, false
	}
	mo, mt := matches(b, o), matches(b, t)
	var out strings.Builder
	pb, po, pt := 0, 0, 0
	// chunk merges the lines between the last line all three share and
	// the next: base[pb:eb], ours[po:eo], theirs[pt:et].
	chunk := func(eb, eo, et int) {
		cb, co, ct := b[pb:eb], o[po:eo], t[pt:et]
		switch {
		case same(co, cb):
			write(&out, ct)
		case same(ct, cb), same(co, ct):
			write(&out, co)
		default:
			conflicts++
			out.WriteString(markerOurs + "\n")
			write(&out, ended(co))
			out.WriteString(markerBase + "\n")
			write(&out, ended(cb))
			out.WriteString(markerSplit + "\n")
			write(&out, ended(ct))
			out.WriteString(markerTheirs + "\n")
		}
	}
	for i := range b {
		// A line of base both sides still have, past what was merged: the
		// parts before it are merged, then it is kept.
		if mo[i] >= po && mt[i] >= pt {
			chunk(i, mo[i], mt[i])
			out.WriteString(b[i])
			pb, po, pt = i+1, mo[i]+1, mt[i]+1
		}
	}
	chunk(len(b), len(o), len(t))
	return []byte(out.String()), conflicts, true
}

// matches is, for each line of x, the line of y it is kept as (from the
// longest common subsequence), or -1.
func matches(x, y []string) []int {
	m := make([]int, len(x))
	for i := range m {
		m[i] = -1
	}
	for _, op := range lineOps(x, y) {
		if op.kind == ' ' {
			m[op.ai-1] = op.bi - 1
		}
	}
	return m
}

func same(a, b []string) bool {
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

func write(out *strings.Builder, lines []string) {
	for _, l := range lines {
		out.WriteString(l)
	}
}

// ended is lines whose last ends with a newline, so a marker after them is
// on a line of its own.
func ended(lines []string) []string {
	if len(lines) == 0 || strings.HasSuffix(lines[len(lines)-1], "\n") {
		return lines
	}
	out := append([]string(nil), lines...)
	out[len(out)-1] += "\n"
	return out
}

// HasConflictMarkers reports whether a file still has a conflict a merge
// left in it.
func HasConflictMarkers(content []byte) bool {
	s := "\n" + string(content)
	return strings.Contains(s, "\n"+markerOurs+"\n") && strings.Contains(s, "\n"+markerTheirs+"\n")
}
