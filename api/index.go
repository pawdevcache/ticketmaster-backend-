package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"ticketmaster/tm"
)

// The handler is built once per warm serverless instance and reused across
// invocations, so the MongoDB connection pool is not rebuilt on every request.
var (
	once    sync.Once
	app     http.Handler
	initErr error
)

// Handler is the Vercel serverless entrypoint. Errors are returned as JSON
// rather than panicking, so a bad config surfaces as a readable 500 instead
// of FUNCTION_INVOCATION_FAILED.
func Handler(w http.ResponseWriter, r *http.Request) {
	// A panic would otherwise kill the function and surface as the opaque
	// FUNCTION_INVOCATION_FAILED, hiding the actual cause.
	defer func() {
		if rec := recover(); rec != nil {
			writeErr(w, fmt.Sprintf("panic: %v", rec))
		}
	}()
	once.Do(func() { app, initErr = tm.New() })
	if initErr != nil {
		writeErr(w, "startup: "+initErr.Error())
		return
	}
	app.ServeHTTP(w, r)
}

func writeErr(w http.ResponseWriter, msg string) {
	log.Println(msg) // shows up in Vercel Runtime Logs
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
