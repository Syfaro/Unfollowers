package main

import (
	"time"
)

// user is a twitter user stored in the database.
type user struct {
	ID int64 `json:"db_id" db:"id"`

	TwitterID   int64  `json:"id" db:"twitter_id"`
	ScreenName  string `json:"screen_name" db:"screen_name"`
	DisplayName string `json:"name" db:"display_name"`

	ProfileIcon string `json:"profile_image_url_https" db:"profile_icon"`
	Color       string `json:"profile_link_color" db:"color"`
}

// userEvent is a user with the most recent event time.
type userEvent struct {
	user
	EventDate time.Time `db:"event_date" json:"date"`
}

// token contains a stored token set and profile data.
type token struct {
	ID int64 `json:"db_id" db:"id"`

	TwitterID   int64  `json:"id" db:"twitter_id"`
	ScreenName  string `json:"screen_name" db:"screen_name"`
	DisplayName string `json:"name" db:"display_name"`

	Token  string `json:"-" db:"token"`
	Secret string `json:"-" db:"secret"`
}

// event is an item such as follow or unfollow.
type event struct {
	ID int64 `db:"id"`

	TokenID int64 `db:"token_id"`
	UserID  int64 `db:"user_id"`

	EventType string    `db:"event_type"`
	EventDate time.Time `db:"event_time"`
}

// userToCheck is a user which is currently being checked.
type userToCheck struct {
	User             interface{}
	IsStillFollowing bool
	IsInDatabase     bool
}

// config is a configuration option from the database.
type config struct {
	ID int64 `db:"id" json:"id"`

	Key   string `db:"key" json:"key"`
	Value string `db:"value" json:"value"`
}
