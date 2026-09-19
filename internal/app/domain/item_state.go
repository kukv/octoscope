package domain

// ItemState is whether a pull request or an issue is still open, translated
// out of the strings GitHub uses so that no view switches on API spelling.
type ItemState int

const (
	StateOpen ItemState = iota
	StateClosed
	StateMerged
)
