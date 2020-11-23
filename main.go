package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")

	if port == "" {
		log.Fatal("$PORT must be set")
	}
	http.HandleFunc("/v1/lobby", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello, %s. X-Forwarded-For: %s, x-Forwarded-Port: %s", r.RemoteAddr, r.Header["X-Forwarded-For"], r.Header["X-Forwarded-Port"])
	})

	log.Fatal(http.ListenAndServe(":"+port, nil))
}
