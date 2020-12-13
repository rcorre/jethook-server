package main

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	_ "github.com/lib/pq"
)

type DB interface {
	Init() error
	GetRecords() ([]record, error)
	PutRecord(record) error
	PutUser(*itchUser) error
}

type db struct {
	*sql.DB
}

func (d *db) Init() error {
	_, err := d.Exec(
		"CREATE TABLE IF NOT EXISTS users(" +
			"itchid integer PRIMARY KEY," +
			"username varchar NOT NULL UNIQUE" +
			")",
	)
	if err != nil {
		return err
	}
	_, err = d.Exec(
		"CREATE TABLE IF NOT EXISTS records(" +
			"itchid integer NOT NULL," +
			"level varchar NOT NULL," +
			"time real NOT NULL," +
			"data bytea NOT NULL," +
			"PRIMARY KEY(itchid, level)" +
			")",
	)
	return err
}

func (d *db) GetRecords() ([]record, error) {
	rows, err := d.Query(
		"SELECT username, level, time, data FROM records " +
			"INNER JOIN users ON records.itchid = users.itchid",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := []record{}
	for rows.Next() {
		var rec record
		var data []byte
		if err := rows.Scan(&rec.UserName, &rec.Level, &rec.Time, &data); err != nil {
			return res, err
		}

		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return res, err
		}
		if rec.Data, err = ioutil.ReadAll(zr); err != nil {
			return res, err
		}
		if err := zr.Close(); err != nil {
			return res, err
		}
		res = append(res, rec)
	}
	return res, rows.Err()
}

func (d *db) PutUser(user *itchUser) error {
	stmt, err := d.Prepare(
		"INSERT INTO users(itchid, username) " +
			"VALUES($1, $2) " +
			"ON CONFLICT (itchid) DO " +
			"UPDATE SET username = excluded.username",
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(user.ID, user.Username)
	return err
}

func (d *db) PutRecord(val record) error {
	// compress to take up less storage
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write(val.Data)
	if err != nil {
		log.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		log.Fatal(err)
	}

	stmt, err := d.Prepare(
		"INSERT INTO records(itchid, level, time, data) " +
			"VALUES($1, $2, $3, $4)" +
			"ON CONFLICT (itchid, level) DO " +
			"UPDATE SET time = excluded.time",
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(val.ItchID, val.Level, val.Time, buf.Bytes())
	return err
}

type v1API struct {
	itchURL string
	db      DB
}

type itchUser struct {
	Username string
	ID       int
}

type record struct {
	ItchID   int `json:"-"`
	UserName string
	Level    string
	Time     float32
	Data     []byte
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

func (v1 *v1API) getRecords(w http.ResponseWriter, r *http.Request) {
	if _, err := v1.getItchUser(r.Header.Get("Authorization")); err != nil {
		log.Printf("Failed to lookup user: %v", err)
		http.Error(w, "", http.StatusUnauthorized)
		return
	}

	records, err := v1.db.GetRecords()
	if err != nil {
		log.Printf("Failed to get records: %v", err)
		http.Error(w, "Failed to get records", http.StatusInternalServerError)
		return
	}

	resp, err := json.Marshal(records)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(resp); err != nil {
		log.Printf("Failed to write response: %v", err)
	}
	log.Println("GET records ok")
}

func (v1 *v1API) postRecord(w http.ResponseWriter, r *http.Request) {
	user, err := v1.getItchUser(r.Header.Get("Authorization"))
	if err != nil {
		log.Printf("Failed to lookup user: %v", err)
		http.Error(w, "", http.StatusUnauthorized)
		return
	}

	if err := v1.db.PutUser(user); err != nil {
		log.Printf("Failed to store user: %v", err)
		http.Error(w, "Failed to store user", http.StatusInternalServerError)
		return
	}

	var entry record
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

	entry.ItchID = user.ID
	if err := v1.db.PutRecord(entry); err != nil {
		log.Printf("Failed to store record: %v", err)
		http.Error(w, "Failed to store record", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	log.Println("POST record ok", entry.UserName, entry.Level, entry.Time)
}

func newMux(itchURL string, db DB) *http.ServeMux {
	mux := http.NewServeMux()
	v1 := &v1API{itchURL: itchURL, db: db}

	mux.HandleFunc("/v1/records", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			v1.getRecords(w, r)
		} else if r.Method == http.MethodPost {
			v1.postRecord(w, r)
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

	pg, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}
	log.Println("Connected to DB")

	d := &db{pg}
	if err := d.Init(); err != nil {
		panic(err)
	}
	log.Println("DB Initialized")

	server := &http.Server{
		Handler: newMux("https://itch.io/api/1/jwt/me", &db{pg}),
		Addr:    ":" + port,
	}
	log.Println("Listening on", port)
	log.Fatal(server.ListenAndServe())
}
