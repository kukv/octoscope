package repo

import (
	"slices"
	"strings"
)

// row is one line of the sidebar: a repository and how much is open in it.
type row struct {
	name string

	// temporary marks the repository the user is standing in when it is not
	// in the settings file. It leads the list but is not part of it: only
	// adding it explicitly writes it there.
	temporary bool

	prs, issues int

	// counted says the badge has an answer. A repository GitHub could not
	// resolve stays uncounted, and the row is drawn without numbers rather
	// than with zeroes.
	counted bool
}

// buildRows turns the settings file's list into the sidebar's rows and picks
// the one to start on. GitHub treats owner/name as case-insensitive, so two
// spellings of one repository fold into the first one written.
func buildRows(repositories []string, current string) ([]row, int) {
	var rows []row
	seen := make(map[string]int, len(repositories))
	for _, name := range repositories {
		key := strings.ToLower(name)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = len(rows)
		rows = append(rows, row{name: name})
	}
	if current == "" {
		return rows, 0
	}
	if i, ok := seen[strings.ToLower(current)]; ok {
		return rows, i
	}
	return append([]row{{name: current, temporary: true}}, rows...), 0
}

// addRow puts name in the list and reports whether anything changed. A row
// already there in any spelling is not repeated -- GitHub treats owner/name
// as case-insensitive -- but a temporary row bearing the name is promoted
// into the list, which is the only way the repository the user is standing
// in ever reaches the settings file.
func addRow(rows []row, name string) ([]row, bool) {
	for i, r := range rows {
		if !strings.EqualFold(r.name, name) {
			continue
		}
		if !r.temporary {
			return rows, false
		}
		rows = slices.Clone(rows)
		rows[i].temporary = false
		return rows, true
	}
	return append(slices.Clone(rows), row{name: name}), true
}

// removeRow drops the row at i and reports whether it was dropped. The
// temporary row is not in the settings file, so there is nothing to remove.
func removeRow(rows []row, i int) ([]row, bool) {
	if i < 0 || i >= len(rows) || rows[i].temporary {
		return rows, false
	}
	return append(slices.Clone(rows[:i]), rows[i+1:]...), true
}

// savedNames is the list as it is written to the settings file: the
// temporary row is the repository the user happens to be standing in, not
// part of the list they are building.
func savedNames(rows []row) []string {
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.temporary {
			continue
		}
		names = append(names, r.name)
	}
	return names
}
