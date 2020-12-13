package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strconv"
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

func tokenToUser(token string) (map[string]interface{}, bool) {
	if !strings.HasPrefix(token, "good") {
		return nil, false
	}
	id, err := strconv.Atoi(strings.TrimPrefix(token, "good"))
	if err != nil {
		panic(err)
	}

	return map[string]interface{}{
		"user": map[string]interface{}{
			"username":     token,
			"gamer":        true,
			"display_name": "Amos",
			"cover_url":    "https://img.itch.zone/aW1hZ2UyL3VzZXIvMjk3ODkvNjkwOTAxLnBuZw==/100x100%23/JkrN%2Bv.png",
			"url":          "https://fasterthanlime.itch.io",
			"press_user":   true,
			"developer":    true,
			"id":           id,
		},
	}, true
}

func setup(t *testing.T) http.Handler {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "bad auth: "+auth, http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		user, found := tokenToUser(token)
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
	t.Cleanup(func() { pg.Exec("DROP TABLE IF EXISTS records") })

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
	r.Header.Set("Authorization", "Bearer good123")
	setup(t).ServeHTTP(w, r)
	expectEq(t, w.Code, http.StatusOK)
	expectEq(t, w.Body.String(), "[]") // no records yet
}

func TestV1Records(t *testing.T) {
	v1 := setup(t)
	get := func() []record {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/records", nil)
		r.Header.Set("Authorization", "Bearer good123")
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
		r.Header.Set("Authorization", "Bearer good123")
		v1.ServeHTTP(w, r)
		expectEq(t, w.Code, http.StatusOK)
		expectEq(t, w.Body.String(), "")
	}

	// no records yet
	expectElementsEq(t, get(), []record{})

	// one record
	post(map[string]interface{}{
		"Level": "levelone",
		"Time":  123.456,
		"Data":  []byte{1, 2, 3, 4, 5},
	})
	expectElementsEq(t, get(), []record{{UserName: "Amos", Level: "levelone", Time: 123.456}})

	// second record, new level
	post(map[string]interface{}{
		"Level": "leveltwo",
		"time":  234.567,
		"Data":  []byte{22, 33, 45, 64},
	})
	expectElementsEq(t, get(), []record{{
		UserName: "Amos",
		Level:    "levelone",
		Time:     123.456,
		Data:     []byte{1, 2, 3, 4, 5},
	}, {
		UserName: "Amos",
		Level:    "leveltwo",
		Time:     234.567,
		Data:     []byte{22, 33, 45, 64},
	}})

	// update first record
	post(map[string]interface{}{
		"Level": "levelone",
		"Time":  12.34,
		"Data":  []byte{27, 23, 15, 44},
	})
	expectElementsEq(t, get(), []record{{
		UserName: "Amos",
		Level:    "levelone",
		Time:     12.34,
		Data:     []byte{27, 23, 15, 44},
	}, {
		UserName: "Amos",
		Level:    "leveltwo",
		Time:     234.567,
		Data:     []byte{22, 33, 45, 64},
	}})
}

func TestV1RecordsOrdering(t *testing.T) {
	v1 := setup(t)
	get := func() []record {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/records", nil)
		r.Header.Set("Authorization", "Bearer good123")
		v1.ServeHTTP(w, r)
		expectEq(t, w.Code, http.StatusOK)
		var res []record
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Error(err)
		}
		return res
	}
	post := func(time float32, idx int) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/v1/records", bytes.NewReader(toJSON(map[string]interface{}{
			"Level": "levelone",
			"Time":  time,
			"Data":  []byte{1, 2, 3, 4, 5},
		})))
		r.Header.Set("Authorization", fmt.Sprintf("Bearer good%d", idx))
		v1.ServeHTTP(w, r)
		expectEq(t, w.Code, http.StatusOK)
		expectEq(t, w.Body.String(), "")
	}

	times := []float32{123.45, 54.32, 12.23, 12.23, 12.23, 13.26, 17.23, 42.16, 142.16, 112.52, 11.12, 107.42, 64.27}
	// should be top 10
	expected := []float32{11.12, 12.23, 12.23, 12.23, 13.26, 17.23, 42.16, 54.32, 64.27, 107.42}
	var actual []float32

	for i, e := range times {
		post(e, i)
	}
	for _, act := range get() {
		actual = append(actual, act.Time)
	}
	expectEq(t, len(actual), len(expected))
	for i := range actual {
		expectEq(t, actual[i], expected[i])
	}
}
