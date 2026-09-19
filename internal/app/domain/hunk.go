package domain

// Hunk is one @@ block. Header is the whole @@ line as git wrote it,
// including the function context git appends after the second @@.
type Hunk struct {
	Header string
	Lines  []DiffLine
}
