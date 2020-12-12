package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"net/http"

	_ "github.com/lib/pq"
)

func toJSON(data interface{}) []byte {
	b, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	return b
}

var tokenToUser map[string]interface{} = map[string]interface{}{
	// example from https://itch.io/docs/api/serverside#reference/profileme-httpsitchioapi1keyme
	"abc123": map[string]interface{}{
		"user": map[string]interface{}{
			"username":     "fasterthanlime",
			"gamer":        true,
			"display_name": "Amos",
			"cover_url":    "https://img.itch.zone/aW1hZ2UyL3VzZXIvMjk3ODkvNjkwOTAxLnBuZw==/100x100%23/JkrN%2Bv.png",
			"url":          "https://fasterthanlime.itch.io",
			"press_user":   true,
			"developer":    true,
			"id":           29789,
		},
	},
}

func setup(t *testing.T) http.Handler {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "bad auth: "+auth, http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		user, found := tokenToUser[token]
		if !found {
			http.Error(w, "bad auth: "+auth, http.StatusUnauthorized)
			return
		}
		if _, err := w.Write(toJSON(user)); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}))
	t.Cleanup(ts.Close)

	pg, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}

	d := &db{pg}
	if err := d.Init(); err != nil {
		panic(err)
	}
	return newMux(ts.URL, d)
}

func expectEq(t *testing.T, a, b interface{}) {
	if a != b {
		t.Errorf("%+v != %+v", a, b)
	}
}

func TestV1AuthMissing(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/leaderboards", nil)
	setup(t).ServeHTTP(w, r)
	expectEq(t, w.Code, http.StatusUnauthorized)
}

func TestV1AuthNoBearer(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/leaderboards", nil)
	r.Header.Set("Authorization", "blah")
	setup(t).ServeHTTP(w, r)
	expectEq(t, w.Code, http.StatusUnauthorized)
}

func TestV1AuthInvalid(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/leaderboards", nil)
	r.Header.Set("Authorization", "Bearer blah")
	setup(t).ServeHTTP(w, r)
	expectEq(t, w.Code, http.StatusUnauthorized)
}

func TestV1AuthOk(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/leaderboards", nil)
	r.Header.Set("Authorization", "Bearer abc123")
	setup(t).ServeHTTP(w, r)
	expectEq(t, w.Code, http.StatusOK)
	expectEq(t, w.Body.String(), "[]") // no leaderboards yet
}

func TestV1Leaderboards(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/leaderboards", nil)
	r.Header.Set("Authorization", "Bearer abc123")
	setup(t).ServeHTTP(w, r)
	expectEq(t, w.Code, http.StatusOK)
	expectEq(t, w.Body.String(), "[]") // no leaderboards yet

	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/v1/leaderboards", bytes.NewReader(toJSON(
		map[string]interface{}{
			"Level": "levelone",
			"time":  123.456,
		},
	)))
	r.Header.Set("Authorization", "Bearer abc123")
	setup(t).ServeHTTP(w, r)
	expectEq(t, w.Code, http.StatusOK)
	expectEq(t, w.Body.String(), "")

	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/v1/leaderboards", nil)
	r.Header.Set("Authorization", "Bearer abc123")
	setup(t).ServeHTTP(w, r)
	expectEq(t, w.Code, http.StatusOK)
	expectEq(t, w.Body.String(), `[{"UserName":"Amos","Level":"levelone","Time":123.456}]`)
}
