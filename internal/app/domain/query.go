package domain

// SavedQuery is one of the Search tab's saved queries: what the user called
// it, and the search it stands for.
//
// Query is opaque to this application. It is the service's own search
// syntax, and octoscope never reads into it: the Search tab writes one from
// its filters (or takes what the user typed), stores it, and hands it back
// unchanged when the user picks that row. Nothing parses one into filters
// again. That is why it is a string rather than a parsed structure, and why
// a query saved against one service does not carry to another.
type SavedQuery struct {
	Name  string
	Query string
}
