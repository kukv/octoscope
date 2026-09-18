package domain

import "time"

type Issue struct {
	Number    int
	Title     string
	Author    Author
	State     ItemState
	UpdatedAt time.Time
	URL       string
	Body      string
	Comments  []Comment
	Labels    []Label
	Assignees []Author
}
