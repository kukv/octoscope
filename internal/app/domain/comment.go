package domain

import "time"

type Comment struct {
	Author    Author
	Body      string
	CreatedAt time.Time
}
