package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

var leaderboards []v1LeaderboardEntry = []v1LeaderboardEntry{}

type v1API struct {
	itchURL string
}

type itchUser struct {
	Username    string
	DisplayName string `json:"display_name"`
	ID          int
}

func (u *itchUser) Name() string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Username
}

type v1LeaderboardEntry struct {
	UserID   int `json:"-"`
	UserName string
	Level    string
	Time     float32
}

func unmarshal(r io.Reader, out interface{}) error {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return fmt.Errorf("Failed to unmarshal %q: %v", b, err)
	}
	return nil
}

func (v1 *v1API) getItchUser(auth string) (*itchUser, error) {
	if !strings.HasPrefix(auth, "Bearer ") {
		return nil, fmt.Errorf("Missing bearer in %q", auth)
	}

	client := &http.Client{}

	req, err := http.NewRequest("GET", v1.itchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("http.NewRequest failed: %v", err)
	}
	req.Header.Add("Authorization", auth)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("client.Do failed: %v", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("itch request error: %s", resp.Status)
	}

	var response struct{ User itchUser }
	if err := unmarshal(resp.Body, &response); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal itch response: %v", err)
	}

	return &response.User, nil
}

func (v1 *v1API) getLeaderboards(w http.ResponseWriter, r *http.Request) {
	if _, err := v1.getItchUser(r.Header.Get("Authorization")); err != nil {
		log.Printf("Failed to lookup user: %v", err)
		http.Error(w, "", http.StatusUnauthorized)
		return
	}

	resp, err := json.Marshal(leaderboards)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(resp); err != nil {
		log.Printf("Failed to write response: %v", err)
	}
}

func (v1 *v1API) postLeaderboards(w http.ResponseWriter, r *http.Request) {
	user, err := v1.getItchUser(r.Header.Get("Authorization"))
	if err != nil {
		log.Printf("Failed to lookup user: %v", err)
		http.Error(w, "", http.StatusUnauthorized)
		return
	}

	var entry v1LeaderboardEntry
	if err := unmarshal(r.Body, &entry); err != nil {
		log.Printf("Failed to parse POST body: %v", err)
		http.Error(w, "", http.StatusBadRequest)
		return
	}
	if entry.Level == "" || entry.Time <= 0 {
		log.Println("Missing time or level")
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	entry.UserName = user.Name()
	entry.UserID = user.ID
	leaderboards = append(leaderboards, entry)
	w.WriteHeader(http.StatusOK)
}

func newMux(itchURL string) *http.ServeMux {
	mux := http.NewServeMux()
	v1 := &v1API{itchURL: itchURL}

	mux.HandleFunc("/v1/leaderboards", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			v1.getLeaderboards(w, r)
		} else if r.Method == http.MethodPost {
			v1.postLeaderboards(w, r)
		} else {
			http.Error(w, "", http.StatusMethodNotAllowed)
		}
	})

	return mux
}

func main() {
	port := os.Getenv("PORT")

	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Handler: newMux("https://itch.io/api/1/jwt/me"),
		Addr:    ":" + port,
	}
	log.Fatal(server.ListenAndServe())
}
