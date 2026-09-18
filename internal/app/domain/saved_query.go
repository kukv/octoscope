package domain

// SavedQuery is one of the Search tab's saved queries: what the user called
// it, and the search it stands for. The query is the service's own search
// syntax, which is why it is a string and not a parsed structure.
type SavedQuery struct {
	Name  string
	Query string
}
