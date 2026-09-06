package detail

import (
	"errors"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

func TestNewPickerPrechecksCurrent(t *testing.T) {
	p := newPicker(pickLabels, "Labels", []string{"bug", "wip"}, map[string]string{"bug": "d73a4a"}, []string{"bug"})
	if len(p.items) != 2 {
		t.Fatalf("items = %d, want 2", len(p.items))
	}
	if p.items[0].name != "bug" || !p.items[0].selected {
		t.Errorf("item0 = %+v, want bug selected", p.items[0])
	}
	if p.items[1].name != "wip" || p.items[1].selected {
		t.Errorf("item1 = %+v, want wip unselected", p.items[1])
	}
	if p.items[0].color != "d73a4a" {
		t.Errorf("color = %q, want d73a4a", p.items[0].color)
	}
}

func TestNewPickerIncludesCurrentNotInCandidates(t *testing.T) {
	// "bug" is currently applied but no longer in the candidate list; it must
	// still appear (selected) so enter does not silently remove it.
	p := newPicker(pickLabels, "Labels", []string{"wip"}, nil, []string{"bug"})
	var names []string
	for _, it := range p.items {
		names = append(names, it.name)
	}
	if len(p.items) != 2 {
		t.Fatalf("items = %v, want wip + bug", names)
	}
	found := false
	for _, it := range p.items {
		if it.name == "bug" && it.selected {
			found = true
		}
	}
	if !found {
		t.Errorf("current-but-uncandidate 'bug' missing or unselected: %v", names)
	}
}

func TestPickerToggleDiff(t *testing.T) {
	p := newPicker(pickLabels, "Labels", []string{"bug", "wip"}, nil, []string{"bug"})
	// cursor at 0 (bug, selected) -> toggle off (remove bug)
	p.toggle()
	// move to wip and toggle on (add wip)
	p.moveDown(10)
	p.toggle()
	add, remove := p.diff()
	if len(add) != 1 || add[0] != "wip" {
		t.Errorf("add = %v, want [wip]", add)
	}
	if len(remove) != 1 || remove[0] != "bug" {
		t.Errorf("remove = %v, want [bug]", remove)
	}
}

func TestPickerNoChangeEmptyDiff(t *testing.T) {
	p := newPicker(pickLabels, "Labels", []string{"bug", "wip"}, nil, []string{"bug"})
	add, remove := p.diff()
	if len(add) != 0 || len(remove) != 0 {
		t.Errorf("diff = %v/%v, want empty", add, remove)
	}
}

func TestPickerCursorAndScroll(t *testing.T) {
	names := []string{"a", "b", "c", "d", "e"}
	p := newPicker(pickAssignees, "Assignees", names, nil, nil)
	// visible window of 2: moving down past the window advances offset
	for range 4 {
		p.moveDown(2)
	}
	if p.cursor != 4 {
		t.Errorf("cursor = %d, want 4", p.cursor)
	}
	if p.offset != 3 { // window [3,5) shows cursor 4
		t.Errorf("offset = %d, want 3", p.offset)
	}
	p.moveDown(2) // already at last item, no-op
	if p.cursor != 4 {
		t.Errorf("cursor moved past end: %d", p.cursor)
	}
	for range 5 {
		p.moveUp()
	}
	if p.cursor != 0 || p.offset != 0 {
		t.Errorf("cursor/offset = %d/%d, want 0/0", p.cursor, p.offset)
	}
}

func TestPickerListViewShowsItems(t *testing.T) {
	p := newPicker(pickLabels, "Labels", []string{"bug", "wip"}, nil, []string{"bug"})
	view := p.listView(20, 80)
	for _, want := range []string{"Labels", "[x] bug", "[ ] wip"} {
		if !strings.Contains(view, want) {
			t.Errorf("listView missing %q:\n%s", want, view)
		}
	}
}

// openPicker loads the detail, presses k and settles the candidate fetch.
func openPicker(t *testing.T, f *fakeSource, ref gh.ItemRef, k string) Model {
	t.Helper()
	m := loaded(f, ref)
	m, cmd := m.Update(key(k))
	if cmd == nil {
		t.Fatalf("cmd = nil after %s, want a candidate fetch", k)
	}
	m, _ = m.Update(cmd())
	return m
}

func TestLOpensLabelPickerPrechecked(t *testing.T) {
	f := &fakeSource{
		pr:     gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen, Labels: []gh.Label{{Name: "bug"}}},
		labels: []gh.Label{{Name: "bug"}, {Name: "wip"}},
	}
	m := loaded(f, prRef())
	m, cmd := m.Update(key("l"))
	if !m.pickerLoading || cmd == nil {
		t.Fatalf("pickerLoading = %v, cmd = %v; want loading with fetch cmd", m.pickerLoading, cmd)
	}
	m, _ = m.Update(cmd()) // pickerCandidatesMsg
	if !m.picking || m.pickerLoading {
		t.Fatalf("picking = %v, pickerLoading = %v; want picking", m.picking, m.pickerLoading)
	}
	if m.picker.kind != pickLabels || len(m.picker.items) != 2 {
		t.Fatalf("picker = %+v, want 2 label items", m.picker)
	}
	if !m.picker.items[0].selected { // "bug" precheck
		t.Errorf("current label not prechecked: %+v", m.picker.items)
	}
}

func TestAOpensAssigneePicker(t *testing.T) {
	f := &fakeSource{
		pr:    gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen},
		users: []string{"alice", "bob"},
	}
	m := openPicker(t, f, prRef(), "a")
	if !m.picking || m.picker.kind != pickAssignees || len(m.picker.items) != 2 {
		t.Fatalf("picker = %+v, want 2 assignee items", m.picker)
	}
}

func TestPickerApplyComputesDiffAndRefetches(t *testing.T) {
	f := &fakeSource{
		pr:     gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen, Labels: []gh.Label{{Name: "bug"}}},
		labels: []gh.Label{{Name: "bug"}, {Name: "wip"}},
	}
	m := openPicker(t, f, prRef(), "l")
	// toggle bug off (cursor 0), move to wip, toggle on
	m, _ = m.Update(key("space"))
	m, _ = m.Update(key("j"))
	m, _ = m.Update(key("space"))
	m, cmd := m.Update(key("enter"))
	if !m.applying || cmd == nil {
		t.Fatalf("applying = %v, cmd = %v; want applying with edit cmd", m.applying, cmd)
	}
	msg := cmd()
	if _, ok := msg.(pickerAppliedMsg); !ok {
		t.Fatalf("msg = %T, want pickerAppliedMsg", msg)
	}
	if len(f.editCalls) != 1 || f.editCalls[0] != "pr:labels::1:add=wip:remove=bug" {
		t.Fatalf("editCalls = %v, want [pr:labels::1:add=wip:remove=bug]", f.editCalls)
	}
	m, cmd = m.Update(msg)
	if m.picking || m.applying || !m.loading || cmd == nil {
		t.Errorf("after applied: picking=%v applying=%v loading=%v cmd=%v; want false,false,true,non-nil",
			m.picking, m.applying, m.loading, cmd)
	}
}

func TestPickerNoChangeClosesWithoutEdit(t *testing.T) {
	f := &fakeSource{
		pr:     gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen, Labels: []gh.Label{{Name: "bug"}}},
		labels: []gh.Label{{Name: "bug"}, {Name: "wip"}},
	}
	m := openPicker(t, f, prRef(), "l")
	m, cmd := m.Update(key("enter")) // no toggle
	if m.picking {
		t.Errorf("picking = true, want closed after empty-diff enter")
	}
	if cmd != nil {
		t.Errorf("cmd = non-nil, want nil for empty diff")
	}
	if len(f.editCalls) != 0 {
		t.Errorf("editCalls = %v, want none", f.editCalls)
	}
}

func TestPickerEscCancels(t *testing.T) {
	f := &fakeSource{
		pr:     gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen, Labels: []gh.Label{{Name: "bug"}}},
		labels: []gh.Label{{Name: "bug"}, {Name: "wip"}},
	}
	m := openPicker(t, f, prRef(), "l")
	m, _ = m.Update(key("space")) // change something
	m, cmd := m.Update(key("esc"))
	if m.picking {
		t.Errorf("picking = true after esc, want false")
	}
	if cmd != nil {
		t.Errorf("cmd = non-nil after esc, want nil (esc closes the picker, not the view)")
	}
	if len(f.editCalls) != 0 {
		t.Errorf("editCalls = %v, want none after esc", f.editCalls)
	}
}

func TestPickerApplyErrorKeepsPicker(t *testing.T) {
	f := &fakeSource{
		pr:      gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen, Labels: []gh.Label{{Name: "bug"}}},
		labels:  []gh.Label{{Name: "bug"}, {Name: "wip"}},
		editErr: errors.New("gh pr: HTTP 403 forbidden"),
	}
	m := openPicker(t, f, prRef(), "l")
	m, _ = m.Update(key("space")) // toggle bug off -> a diff
	m, cmd := m.Update(key("enter"))
	m, _ = m.Update(cmd()) // pickErrorMsg
	if !m.picking {
		t.Errorf("picking = false, want still picking after apply error")
	}
	if m.applying {
		t.Errorf("applying = true, want false after error")
	}
	if !strings.Contains(m.picker.err, "403") {
		t.Errorf("picker.err = %q, want to contain 403", m.picker.err)
	}
	if !strings.Contains(m.View(), "403") {
		t.Errorf("picker view missing error text:\n%s", m.View())
	}
}

func TestPickerFetchErrorInlineOnDetail(t *testing.T) {
	f := &fakeSource{
		pr:        gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen},
		labelsErr: errors.New("gh label: HTTP 403 forbidden"),
	}
	m := loaded(f, prRef())
	m, cmd := m.Update(key("l"))
	m, cmd = m.Update(cmd()) // pickErrorMsg (picking was never set)
	if m.picking || m.pickerLoading {
		t.Errorf("picking/pickerLoading = %v/%v, want false", m.picking, m.pickerLoading)
	}
	if cmd != nil {
		t.Errorf("cmd = non-nil, want nil (a candidate fetch failure stays inline)")
	}
	if !strings.Contains(m.actionErr, "403") {
		t.Errorf("actionErr = %q, want to contain 403", m.actionErr)
	}
}

func TestPickerApplyOnIssueRoutesToIssue(t *testing.T) {
	f := &fakeSource{
		issue:  gh.Issue{Number: 5, Title: "an issue", State: gh.StateOpen},
		labels: []gh.Label{{Name: "bug"}},
	}
	m := openPicker(t, f, issueRef(), "l")
	m, _ = m.Update(key("space")) // add bug
	_, cmd := m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("cmd = nil, want edit cmd")
	}
	cmd()
	if len(f.editCalls) != 1 || f.editCalls[0] != "issue:labels::5:add=bug:remove=" {
		t.Errorf("editCalls = %v, want [issue:labels::5:add=bug:remove=]", f.editCalls)
	}
}

func TestPickerApplyAssigneesRoutesToPR(t *testing.T) {
	f := &fakeSource{
		pr:    gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen},
		users: []string{"alice"},
	}
	m := openPicker(t, f, prRef(), "a")
	m, _ = m.Update(key("space")) // select alice -> add
	_, cmd := m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("cmd = nil, want edit cmd")
	}
	cmd()
	if len(f.editCalls) != 1 || f.editCalls[0] != "pr:assignees::1:add=alice:remove=" {
		t.Errorf("editCalls = %v, want [pr:assignees::1:add=alice:remove=]", f.editCalls)
	}
}

func TestPickerLoadingIgnoresKeys(t *testing.T) {
	f := &fakeSource{
		pr:     gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen},
		labels: []gh.Label{{Name: "bug"}, {Name: "wip"}},
	}
	m := loaded(f, prRef())
	m, _ = m.Update(key("l"))
	if !m.pickerLoading {
		t.Fatalf("pickerLoading = false, want true right after pressing l")
	}
	m, cmd := m.Update(key("x")) // fetch still in flight; must be a no-op
	if cmd != nil {
		t.Errorf("cmd = %v, want nil while pickerLoading", cmd)
	}
	if m.confirming || m.composing || m.picking {
		t.Errorf("confirming=%v composing=%v picking=%v, want all false while pickerLoading",
			m.confirming, m.composing, m.picking)
	}
}

func TestPickerIgnoresKeysWhileApplying(t *testing.T) {
	f := &fakeSource{
		pr:     gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen, Labels: []gh.Label{{Name: "bug"}}},
		labels: []gh.Label{{Name: "bug"}, {Name: "wip"}},
	}
	m := openPicker(t, f, prRef(), "l")
	m, _ = m.Update(key("space"))
	m, _ = m.Update(key("enter")) // applying == true (the cmd is deliberately not run)
	if !m.applying {
		t.Fatalf("precondition: applying = false, want true")
	}
	m, cmd := m.Update(key("esc"))
	if cmd != nil {
		t.Errorf("cmd = non-nil while applying, want nil")
	}
	if !m.applying || !m.picking {
		t.Errorf("applying/picking changed while applying: applying=%v picking=%v", m.applying, m.picking)
	}
}

func TestPickerViewShowsItemsAndHelp(t *testing.T) {
	f := &fakeSource{
		pr:     gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen, Labels: []gh.Label{{Name: "bug"}}},
		labels: []gh.Label{{Name: "bug"}, {Name: "wip"}},
	}
	m := openPicker(t, f, prRef(), "l")
	view := m.View()
	for _, want := range []string{"Labels", "[x] bug", "[ ] wip", "space:toggle", "enter:apply", "esc:cancel"} {
		if !strings.Contains(view, want) {
			t.Errorf("picker view missing %q:\n%s", want, view)
		}
	}
}
