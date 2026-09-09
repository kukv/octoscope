package repo

import "strings"

// row is one line of the sidebar: a repository and how much is open in it.
type row struct {
	name string

	// temporary marks the repository the user is standing in when it is not
	// in the settings file. It leads the list but is not part of it: only
	// adding it explicitly writes it there.
	temporary bool
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
