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

	if _, err := pg.Exec("DROP TABLE IF EXISTS records"); err != nil {
		panic(err)
	}

	d := &db{pg}
	if err := d.Init(); err != nil {
		panic(err)
	}
	return newMux(ts.URL, d)
}

func expectElementsEq(t *testing.T, a, b []record) {
	if len(a) != len(b) {
		t.Errorf("len(a) = %d, len(b) = %d", len(a), len(b))
	}
	for x := range a {
		found := false
		for y := range b {
			if x == y {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("No match for %v in %v", x, b)
		}
	}
}

func expectEq(t *testing.T, a, b interface{}) {
	if a != b {
		t.Errorf("%+v != %+v", a, b)
	}
}

func TestV1AuthMissing(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/records", nil)
	setup(t).ServeHTTP(w, r)
	expectEq(t, w.Code, http.StatusUnauthorized)
}

func TestV1AuthNoBearer(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/records", nil)
	r.Header.Set("Authorization", "blah")
	setup(t).ServeHTTP(w, r)
	expectEq(t, w.Code, http.StatusUnauthorized)
}

func TestV1AuthInvalid(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/records", nil)
	r.Header.Set("Authorization", "Bearer blah")
	setup(t).ServeHTTP(w, r)
	expectEq(t, w.Code, http.StatusUnauthorized)
}

func TestV1AuthOk(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/records", nil)
	r.Header.Set("Authorization", "Bearer abc123")
	setup(t).ServeHTTP(w, r)
	expectEq(t, w.Code, http.StatusOK)
	expectEq(t, w.Body.String(), "[]") // no records yet
}

func TestV1Records(t *testing.T) {
	// TODO: expect contains same
	v1 := setup(t)
	get := func() []record {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/records", nil)
		r.Header.Set("Authorization", "Bearer abc123")
		v1.ServeHTTP(w, r)
		expectEq(t, w.Code, http.StatusOK)
		var res []record
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Error(err)
		}
		return res
	}
	post := func(data map[string]interface{}) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/records", bytes.NewReader(toJSON(data)))
		r.Header.Set("Authorization", "Bearer abc123")
		v1.ServeHTTP(w, r)
		expectEq(t, w.Code, http.StatusOK)
		expectEq(t, w.Body.String(), "")
	}

	// no records yet
	expectElementsEq(t, get(), []record{})

	// one record
	post(map[string]interface{}{
		"Level": "levelone",
		"time":  123.456,
	})
	expectElementsEq(t, get(), []record{{UserName: "Amos", Level: "levelone", Time: 123.456}})

	// second record, new level
	post(map[string]interface{}{
		"Level": "leveltwo",
		"time":  234.567,
	})
	expectElementsEq(t, get(), []record{{
		UserName: "Amos",
		Level:    "levelone",
		Time:     123.456,
	}, {
		UserName: "Amos",
		Level:    "leveltwo",
		Time:     234.567,
	}})

	// update first record
	post(map[string]interface{}{
		"Level": "levelone",
		"time":  12.34,
	})
	expectElementsEq(t, get(), []record{{
		UserName: "Amos",
		Level:    "levelone",
		Time:     12.34,
	}, {
		UserName: "Amos",
		Level:    "leveltwo",
		Time:     234.567,
	}})
}
