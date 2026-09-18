package domain

import "time"

// ThreadComment is one comment inside a review thread. Pending marks a
// comment the viewer has written but not submitted: GitHub returns it in the
// same place as everyone else's, and only the state of the review it belongs
// to tells the two apart.
type ThreadComment struct {
	Author    Author
	Body      string
	CreatedAt time.Time
	Pending   bool
}
