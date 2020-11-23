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
		fmt.Fprintf(w, "Hello: %+v", r.Header)
	})

	log.Fatal(http.ListenAndServe(":"+port, nil))
}
