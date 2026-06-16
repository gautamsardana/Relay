package models

import "time"

type User struct {
	UserID    string
	Email     string
	CreatedAt time.Time
}
