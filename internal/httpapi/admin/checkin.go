package admin

import (
	"net/http"

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
	result, booking, err := h.AdminStore.CheckIn(req.Code)
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
