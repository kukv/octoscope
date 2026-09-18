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

// Work holds the items of each column, indexed by WorkSection.
type Work [WorkSectionCount][]WorkItem
