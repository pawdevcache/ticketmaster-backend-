package admin

import (
	"net/http"
	"time"

	"ticketmaster/internal/httpapi/web"
	adminstore "ticketmaster/internal/store/admin"
)

// --- admin: ticket check-in ---

// CheckIn admits a ticket scanned at the door.
//
// The code arrives in the body rather than the path because it is the ticket's
// only credential: URLs end up in access logs, browser history and referrer
// headers, and a leaked code is a free entry.
//
// Every outcome carries a machine-readable result and, where one exists, the
// booking — a scanner needs to show the holder's event and seat count, and to
// distinguish "already used at 19:42" from "no such ticket".
func (h *Handlers) CheckIn(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if web.ReadJSON(w, r, &req) != nil || req.Code == "" {
		web.Fail(w, http.StatusBadRequest, "ticket code required")
		return
	}
	result, booking, err := h.AdminStore.CheckIn(req.Code, time.Time{})
	if err != nil {
		web.ServerError(w, err)
		return
	}

	// Refusals are 4xx so a scanner can react to the status alone, but the
	// body always explains which refusal it was.
	status := http.StatusOK
	switch result {
	case adminstore.CheckInUnknown:
		status = http.StatusNotFound
	case adminstore.CheckInUsed, adminstore.CheckInCancelled, adminstore.CheckInUnpaid:
		status = http.StatusConflict
	}
	out := map[string]any{"result": result, "message": checkInMessage(result)}
	if booking != nil {
		out["booking"] = booking
	}
	web.WriteJSON(w, status, out)
}

// checkInMessage is wording a gate attendant can act on without decoding the
// result constant.
func checkInMessage(r adminstore.CheckInResult) string {
	switch r {
	case adminstore.CheckInValid:
		return "admitted"
	case adminstore.CheckInUsed:
		return "this ticket has already been scanned"
	case adminstore.CheckInCancelled:
		return "this booking was cancelled"
	case adminstore.CheckInUnpaid:
		return "this booking was never paid for"
	default:
		return "no ticket matches that code"
	}
}

// maxBulkScans bounds one flush. A gate that was offline for an evening still
// fits comfortably; anything larger is a mistake worth rejecting outright.
const maxBulkScans = 1000

// CheckInBulk admits a batch of scans queued by a gate that was offline.
//
// Each scan carries its own scannedAt, so a ticket records the moment it went
// through the door rather than the moment the network came back. Outcomes are
// per scan and the response is always 200: one refused ticket in a batch of
// three hundred is a fact to report, not a failed request.
func (h *Handlers) CheckInBulk(w http.ResponseWriter, r *http.Request) {
	if h.AdminAuth(w, r) == nil {
		return
	}
	var req struct {
		Scans []struct {
			Code      string    `json:"code"`
			ScannedAt time.Time `json:"scannedAt"`
		} `json:"scans"`
	}
	if web.ReadJSON(w, r, &req) != nil || len(req.Scans) == 0 {
		web.Fail(w, http.StatusBadRequest, "scans required")
		return
	}
	if len(req.Scans) > maxBulkScans {
		web.Fail(w, http.StatusRequestEntityTooLarge, "too many scans in one batch")
		return
	}
	results := make([]map[string]any, 0, len(req.Scans))
	summary := map[string]int{}
	for _, s := range req.Scans {
		row := map[string]any{"code": s.Code}
		result, b, err := h.AdminStore.CheckIn(s.Code, s.ScannedAt)
		if err != nil {
			// Keep going: one database hiccup must not discard the rest of a
			// flush that a gate cannot easily replay.
			result = "error"
			row["message"] = "could not be processed, retry this scan"
		} else {
			row["message"] = checkInMessage(result)
			if b != nil {
				row["bookingId"] = b.ID
			}
		}
		row["result"] = result
		summary[string(result)]++
		results = append(results, row)
	}
	web.WriteJSON(w, http.StatusOK, map[string]any{"results": results, "summary": summary})
}
