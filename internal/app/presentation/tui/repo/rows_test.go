package repo

import "testing"

func names(rows []row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.name
	}
	return out
}

func TestBuildRowsKeepsSettingsOrder(t *testing.T) {
	rows, selected := buildRows([]string{"kukv/octoscope", "kukv/koto"}, "")
	if got := names(rows); got[0] != "kukv/octoscope" || got[1] != "kukv/koto" || len(got) != 2 {
		t.Errorf("rows = %v, want the settings order", got)
	}
	if selected != 0 {
		t.Errorf("selected = %d, want 0", selected)
	}
}

// The settings file is hand-edited, and GitHub treats owner/name as
// case-insensitive: two spellings of one repository must not become two rows.
func TestBuildRowsFoldsDuplicates(t *testing.T) {
	rows, _ := buildRows([]string{"kukv/octoscope", "KUKV/Octoscope", "kukv/koto"}, "")
	if got := names(rows); len(got) != 2 || got[1] != "kukv/koto" {
		t.Errorf("rows = %v, want the duplicate folded away", got)
	}
}

// The current repository is not written to the settings file: it leads the
// list as a temporary row until the user adds it.
func TestBuildRowsPrependsCurrentWhenAbsent(t *testing.T) {
	rows, selected := buildRows([]string{"kukv/koto"}, "kukv/octoscope")
	if got := names(rows); len(got) != 2 || got[0] != "kukv/octoscope" {
		t.Fatalf("rows = %v, want the current repository first", got)
	}
	if !rows[0].temporary {
		t.Error("the current repository was recorded as part of the saved list")
	}
	if selected != 0 {
		t.Errorf("selected = %d, want the current repository selected", selected)
	}
}

func TestBuildRowsSelectsCurrentAlreadyInList(t *testing.T) {
	rows, selected := buildRows([]string{"kukv/koto", "KUKV/Octoscope"}, "kukv/octoscope")
	if len(rows) != 2 {
		t.Fatalf("rows = %v, want no extra row", names(rows))
	}
	if selected != 1 {
		t.Errorf("selected = %d, want the row already in the list", selected)
	}
	if rows[1].temporary {
		t.Error("a row from the settings file was marked temporary")
	}
}

func TestBuildRowsWithoutCurrent(t *testing.T) {
	rows, selected := buildRows(nil, "")
	if len(rows) != 0 || selected != 0 {
		t.Errorf("rows = %v, selected = %d, want nothing to show", names(rows), selected)
	}
}
