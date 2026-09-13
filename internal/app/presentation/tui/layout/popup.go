package layout

// borderCols and padCols are what theme.Popup costs around its contents:
// one column of border and one of padding on each side.
const (
	borderCols = 2
	padCols    = 2
)

// PopupWidth keeps a popup readable at eighty columns without letting it run
// the full width of a wide terminal.
func PopupWidth(termCols int) int {
	if termCols <= 0 {
		return 40
	}
	return max(min(termCols-4, 60), 20)
}

// PopupContentWidth is what a line inside the box may occupy. A longer line
// makes lipgloss wrap it, which splits a row across two.
func PopupContentWidth(boxCols int) int {
	return max(boxCols-borderCols-padCols, 1)
}
