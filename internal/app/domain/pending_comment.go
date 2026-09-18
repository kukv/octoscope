package domain

// PendingComment is a line comment on its way to GitHub.
type PendingComment struct {
	Path string
	Line int
	Side DiffSide
	Body string
}
