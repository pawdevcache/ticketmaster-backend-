package tm

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

func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.Time) }

func (d Date) MarshalBSONValue() (bsontype.Type, []byte, error) { return bson.MarshalValue(d.Time) }

func (d *Date) UnmarshalBSONValue(t bsontype.Type, data []byte) error {
	return bson.UnmarshalValue(t, data, &d.Time)
}
