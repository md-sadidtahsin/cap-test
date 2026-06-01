package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) {
	var err error
	db, err = sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	createTableSQL := `CREATE TABLE IF NOT EXISTS urls (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		short_code TEXT UNIQUE NOT NULL,
		long_url TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		t.Fatal(err)
	}
}

func teardownTestDB() {
	if db != nil {
		db.Close()
	}
}

func TestGetEnvFallback(t *testing.T) {
	os.Unsetenv("TEST_GO_ENV")
	value := getEnv("TEST_GO_ENV", "fallback")
	if value != "fallback" {
		t.Fatalf("expected fallback, got %s", value)
	}
}

func TestGenerateShortCode(t *testing.T) {
	code := generateShortCode()
	if len(code) != 6 {
		t.Fatalf("expected 6 chars, got %d", len(code))
	}
	if strings.ContainsAny(code, "+/=\n") {
		t.Fatalf("unexpected special chars in short code: %s", code)
	}
}

func TestCreateShortURL(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", strings.NewReader(`{"long_url":"http://example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	createShortURL(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp ShortenResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if resp.LongURL != "http://example.com" {
		t.Fatalf("unexpected long_url: %s", resp.LongURL)
	}

	row := db.QueryRow("SELECT long_url FROM urls WHERE short_code = ?", resp.ShortCode)
	var stored string
	if err := row.Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != resp.LongURL {
		t.Fatalf("expected stored URL %s, got %s", resp.LongURL, stored)
	}
}

func TestRedirectNotFound(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: "missing"}}

	redirect(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSendClickEventHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event ClickEvent
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Fatal(err)
		}
		if event.ShortCode != "abc123" {
			t.Fatalf("unexpected short_code: %s", event.ShortCode)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	pythonServiceURL = server.URL
	sendClickEventHTTP("abc123")
}
