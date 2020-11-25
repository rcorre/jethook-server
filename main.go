package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path"
)

// name -> address
var lobbies map[string]string

func v1LobbyList(r *http.Request) ([]byte, int) {
	data, err := json.Marshal(lobbies)
	if err != nil {
		return nil, http.StatusInternalServerError
	}
	return data, http.StatusOK
}

func v1LobbyPut(r *http.Request) ([]byte, int) {
	name := path.Base(r.URL.Path)

	for n, address := range lobbies {
		if n == name || address == r.RemoteAddr {
			return nil, http.StatusConflict
		}
	}

	lobbies[name] = r.RemoteAddr
	return nil, http.StatusOK
}

func v1LobbyDelete(r *http.Request) ([]byte, int) {
	name := path.Base(r.URL.Path)

	if address, ok := lobbies[name]; !ok {
		return nil, http.StatusNotFound
	} else if address != r.RemoteAddr {
		return nil, http.StatusNotFound
	}

	delete(lobbies, name)
	return nil, http.StatusOK
}

func main() {
	port := os.Getenv("PORT")

	if port == "" {
		log.Fatal("$PORT must be set")
	}

	lobbies = map[string]string{}

	http.HandleFunc("/v1/lobby/", func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		var code int

		switch r.Method {
		case http.MethodGet:
			body, code = v1LobbyList(r)
		case http.MethodPut:
			body, code = v1LobbyPut(r)
		case http.MethodDelete:
			body, code = v1LobbyDelete(r)
		default:
			code = http.StatusMethodNotAllowed
		}

		if body != nil {
			if _, err := w.Write(body); err != nil {
				code = http.StatusInternalServerError
			}
		}
		if code >= 300 {
			http.Error(w, http.StatusText(code), code)
		}
	})

	log.Fatal(http.ListenAndServe(":"+port, nil))
}
