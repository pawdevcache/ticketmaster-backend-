package main

import (
	"log"
	"net/http"

	"ticketmaster/internal/tm"
)

// Local development server. On Vercel, api/index.go is the entrypoint instead.
func main() {
	h, err := tm.New()
	if err != nil {
		log.Fatal("startup: ", err)
	}
	addr := ":" + tm.Port()
	log.Println("Ticketmaster API listening on", addr)
	log.Fatal(http.ListenAndServe(addr, h))
}
