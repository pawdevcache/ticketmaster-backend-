package web

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"ticketmaster/internal/store"
)

// The page block is what a client uses to decide whether to ask for another
// page, and totalElements now comes from a count query rather than len(items).
// Getting the arithmetic wrong strands records no client will ever request.
func TestWritePageEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name       string
		items      []string
		total      int64
		page       store.Page
		wantPages  int64
		wantNumber int64
	}{
		{"exact multiple", []string{"a", "b"}, 20, store.Page{Number: 0, Size: 10}, 2, 0},
		{"partial last page", []string{"a"}, 21, store.Page{Number: 2, Size: 10}, 3, 2},
		{"single page", []string{"a"}, 1, store.Page{Number: 0, Size: 20}, 1, 0},
		{"no results", []string{}, 0, store.Page{Number: 0, Size: 20}, 0, 0},
		{"page past the end", []string{}, 5, store.Page{Number: 9, Size: 20}, 1, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WritePage(rec, "things", tc.items, tc.total, tc.page)

			var got struct {
				Embedded map[string][]string `json:"_embedded"`
				Page     map[string]int64    `json:"page"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("response is not valid JSON: %v\n%s", err, rec.Body)
			}
			if got.Page["totalElements"] != tc.total {
				t.Errorf("totalElements = %d, want %d", got.Page["totalElements"], tc.total)
			}
			if got.Page["totalPages"] != tc.wantPages {
				t.Errorf("totalPages = %d, want %d", got.Page["totalPages"], tc.wantPages)
			}
			if got.Page["number"] != tc.wantNumber {
				t.Errorf("number = %d, want %d", got.Page["number"], tc.wantNumber)
			}
			if len(got.Embedded["things"]) != len(tc.items) {
				t.Errorf("item count = %d, want %d", len(got.Embedded["things"]), len(tc.items))
			}
		})
	}
}

// An empty page must serialise as [] — a null would break clients that iterate
// the array without a nil check.
func TestWritePageEmptyIsArrayNotNull(t *testing.T) {
	rec := httptest.NewRecorder()
	WritePage(rec, "things", []string{}, 0, store.Page{Number: 0, Size: 20})
	if body := rec.Body.String(); !contains(body, `"things":[]`) {
		t.Errorf("empty page did not serialise as []: %s", body)
	}
}

// PageParams must clamp an abusive size before it reaches the database.
func TestPageParamsClampsSize(t *testing.T) {
	for _, tc := range []struct {
		query    string
		wantSize int
		wantNum  int
	}{
		{"", store.DefaultPageSize, 0},
		{"?size=10&page=3", 10, 3},
		{"?size=99999", store.MaxPageSize, 0},
		{"?size=-1", store.DefaultPageSize, 0},
		{"?size=abc", store.DefaultPageSize, 0},
		{"?page=-4", store.DefaultPageSize, 0},
	} {
		p := PageParams(httptest.NewRequest("GET", "/x"+tc.query, nil))
		if p.Size != tc.wantSize || p.Number != tc.wantNum {
			t.Errorf("PageParams(%q) = {Number:%d Size:%d}, want {Number:%d Size:%d}",
				tc.query, p.Number, p.Size, tc.wantNum, tc.wantSize)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	}()
}
