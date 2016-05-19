package main

import (
	"database/sql"
	"encoding/json"
)

type nullString struct {
	sql.NullString
}

func (s nullString) MarshalJSON() ([]byte, error) {
	if !s.Valid {
		return json.Marshal(nil)
	}

	return json.Marshal(s.String)
}
