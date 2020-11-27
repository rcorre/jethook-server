package main

import (
	"log"
	"net/http"
	"os"
	"sync"
)

var lock sync.Mutex
var wait chan string
var host string

func v1GetMatch(w http.ResponseWrite, r *http.Request) {
	lock.Lock()

	if wait == nil {
		// no other player, become a host and wait
		host = r.RemoteAddr
		wait = make(chan string)
		lock.Unlock()

		guest := <-wait
	} else {
		// send guest address to the host
		wait <- r.RemoteAddr
		wait = nil
		h := host
		host = nil
		lock.Unlock()

		// send host address to the guest
		w.W:
	}

	player = r.RemoteAddr
	return nil, http.StatusOK
}

func main() {
	port := os.Getenv("PORT")

	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/v1/match", func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		var code int

		if r.Method == http.MethodGet {
			v1GetMatch(w, r)
		} else {
			http.Error(w, http.StatusMethodNotAllowed)
		}
	})

	log.Fatal(http.ListenAndServe(":"+port, nil))
}
