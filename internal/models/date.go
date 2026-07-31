package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
)

// Date wraps time.Time but accepts flexible JSON input: a plain "2006-01-02"
// date or a full RFC3339 timestamp. It stores/loads as a native BSON date so
// Mongo range queries keep working.
type Date struct{ time.Time }

var dateLayouts = []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"}

// UnmarshalJSON accepts any layout in dateLayouts, so a client may send either
// "2026-09-01" or a full timestamp. An empty or null value leaves the zero
// time in place rather than erroring, which is what makes a partial update
// work: omitting the field must not blank the stored date.
func (d *Date) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			d.Time = t
			return nil
		}
	}
	return fmt.Errorf("invalid date %q (use YYYY-MM-DD or RFC3339)", s)
}

// MarshalJSON always writes RFC3339, whatever layout came in.
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.Time) }

// MarshalBSONValue stores a native BSON date rather than a string, so Mongo
// range queries on the field keep working.
func (d Date) MarshalBSONValue() (bsontype.Type, []byte, error) { return bson.MarshalValue(d.Time) }

// UnmarshalBSONValue reads back the native date written by MarshalBSONValue.
func (d *Date) UnmarshalBSONValue(t bsontype.Type, data []byte) error {
	return bson.UnmarshalValue(t, data, &d.Time)
}
