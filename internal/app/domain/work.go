package domain

// WorkSection is one column of the Work board.
type WorkSection int

const (
	SectionReviewRequested WorkSection = iota
	SectionYourPRs
	SectionAssigned
	SectionMentioned

	// WorkSectionCount must be the last constant in this block: Work's
	// length as an array indexed by WorkSection comes from it.
	WorkSectionCount = iota
)

// WorkSections returns the columns in display order, left to right.
func WorkSections() []WorkSection {
	sections := make([]WorkSection, WorkSectionCount)
	for i := range sections {
		sections[i] = WorkSection(i)
	}
	return sections
}

// Work holds the items of each column. The columns are unexported so that
// the range of a section index is this type's business rather than every
// caller's: a WorkSection is the only way in, and there is no index to get
// wrong.
type Work struct {
	sections [WorkSectionCount][]WorkItem
}

// Section is the items of one column, in the order they arrived.
func (w Work) Section(s WorkSection) []WorkItem { return w.sections[s] }

// SetSection replaces one column. A fetch answers for one column at a time,
// and what the others hold is not its business.
func (w *Work) SetSection(s WorkSection, items []WorkItem) { w.sections[s] = items }
