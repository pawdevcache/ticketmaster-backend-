// Command server runs the Ticketmaster API as an ordinary HTTP server, for
// local development and for container or Render deployments. Vercel uses
// api/index.go instead; both build the same handler via httpapi.New.
package main

import (
	"log"
	"net/http"

	"ticketmaster/internal/config"
	"ticketmaster/internal/httpapi"
)

// Local development server. On Vercel, api/index.go is the entrypoint instead.
func main() {
	h, err := httpapi.New()
	if err != nil {
		log.Fatal("startup: ", err)
	}
	addr := ":" + config.Port()
	log.Println("Ticketmaster API listening on", addr)
	log.Fatal(http.ListenAndServe(addr, h))
}
