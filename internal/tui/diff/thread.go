package diff

import (
	"strconv"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

// threadKey names one thread by where it sits. It is the map key for which
// settled threads the user has opened, and it has to survive a refetch, so it
// is built from the position rather than from an id.
func threadKey(path string, line int, side gh.DiffSide) string {
	return path + ":" + strconv.Itoa(line) + ":" + strconv.Itoa(int(side))
}

// threadsFor returns the threads that belong under one line of the diff.
// Path, line and side must all match: a comment on the old version of a line
// is not a comment on the new one.
func (m Model) threadsFor(path string, line int, side gh.DiffSide) []gh.ReviewThread {
	var out []gh.ReviewThread
	for _, t := range m.review.Threads {
		if t.Path == path && t.Line == line && t.Side == side {
			out = append(out, t)
		}
	}
	return out
}

// threadRows turns the threads under one line into drawable rows. Settled
// ones collapse into a single count until the user opens them, so that
// finished arguments do not push the code they were about off the screen.
func (m Model) threadRows(hunk int, path string, line int, side gh.DiffSide) []row {
	threads := m.threadsFor(path, line, side)
	if len(threads) == 0 {
		return nil
	}
	key := threadKey(path, line, side)
	var rows []row
	var settled int
	for _, t := range threads {
		if t.Collapsed() && !m.expanded[key] {
			settled++
			continue
		}
		for _, c := range t.Comments {
			rows = append(rows, row{kind: rowThread, hunk: hunk, key: key, thread: t, comment: c})
		}
	}
	if settled > 0 {
		rows = append(rows, row{
			kind: rowCollapsed, hunk: hunk, key: key,
			text: i18n.Tn("diff.collapsed", settled),
		})
	}
	return rows
}

// orphanRows are the comments whose line this diff does not show: the file is
// not in the diff, or the line has gone. They go at the end of the file they
// name rather than being dropped -- a comment nobody can see is a comment
// nobody answers.
func (m Model) orphanRows(placed map[string]bool) []row {
	var rows []row
	for _, t := range m.review.Threads {
		if t.Path != m.files[m.file].Path {
			continue
		}
		if placed[threadKey(t.Path, t.Line, t.Side)] {
			continue
		}
		rows = append(rows, row{kind: rowNote, hunk: -1, text: i18n.T("diff.orphaned")})
		break
	}
	if len(rows) == 0 {
		return nil
	}
	for _, t := range m.review.Threads {
		if t.Path != m.files[m.file].Path || placed[threadKey(t.Path, t.Line, t.Side)] {
			continue
		}
		for _, c := range t.Comments {
			rows = append(rows, row{kind: rowThread, hunk: -1, thread: t, comment: c})
		}
	}
	return rows
}
