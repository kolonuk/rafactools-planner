package docs

import "strings"

// DiffLine is one line of a comparison.
type DiffLine struct {
	Kind string // "same", "add" or "del"
	Text string
}

// maxDiffLines bounds the O(n*m) table; docs pages are far smaller.
const maxDiffLines = 2000

// Diff returns a line-based diff turning a into b.
func Diff(a, b string) []DiffLine {
	al, bl := splitLines(a), splitLines(b)
	if len(al) > maxDiffLines {
		al = al[:maxDiffLines]
	}
	if len(bl) > maxDiffLines {
		bl = bl[:maxDiffLines]
	}
	// Longest-common-subsequence table.
	lcs := make([][]int, len(al)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(bl)+1)
	}
	for i := len(al) - 1; i >= 0; i-- {
		for j := len(bl) - 1; j >= 0; j-- {
			if al[i] == bl[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var out []DiffLine
	i, j := 0, 0
	for i < len(al) && j < len(bl) {
		switch {
		case al[i] == bl[j]:
			out = append(out, DiffLine{"same", al[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, DiffLine{"del", al[i]})
			i++
		default:
			out = append(out, DiffLine{"add", bl[j]})
			j++
		}
	}
	for ; i < len(al); i++ {
		out = append(out, DiffLine{"del", al[i]})
	}
	for ; j < len(bl); j++ {
		out = append(out, DiffLine{"add", bl[j]})
	}
	return out
}

func splitLines(s string) []string {
	s = strings.TrimRight(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// Changed reports whether any line differs.
func Changed(d []DiffLine) bool {
	for _, l := range d {
		if l.Kind != "same" {
			return true
		}
	}
	return false
}
