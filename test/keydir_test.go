package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/johalputt/VayuMail-Mobile/internal/mail/pgp"
)

// publicArmored generates a key and returns its ASCII-armored public half.
func publicArmored(t *testing.T, name, email string) string {
	t.Helper()
	kr := pgp.NewKeyring()
	fps, err := kr.ImportArmored(newTestKey(t, name, email))
	if err != nil {
		t.Fatal(err)
	}
	pub, err := kr.ExportPublicArmored(fps[0])
	if err != nil {
		t.Fatal(err)
	}
	return string(pub)
}

func TestDiscoverKeyDirectoryPerAddress(t *testing.T) {
	armored := publicArmored(t, "Bob", "bob@johal.in")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("email") != "bob@johal.in" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/pgp-keys")
		_, _ = w.Write([]byte(armored))
	}))
	defer srv.Close()

	ents, err := pgp.DiscoverKeyDirectory(context.Background(), srv.Client(), srv.URL, "bob@johal.in")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(ents) != 1 {
		t.Fatalf("entities = %d, want 1", len(ents))
	}
	kr := pgp.NewKeyring()
	kr.ImportEntities(ents)
	if !kr.HasKeyFor("bob@johal.in") {
		t.Fatal("imported key not usable for bob@johal.in")
	}
}

func TestDiscoverKeyDirectoryMissing(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := pgp.DiscoverKeyDirectory(context.Background(), srv.Client(), srv.URL, "nobody@johal.in"); err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestFetchKeyDirectoryBulk(t *testing.T) {
	keys := []pgp.DirectoryKey{
		{Email: "a@johal.in", Armored: publicArmored(t, "A", "a@johal.in")},
		{Email: "b@johal.in", Armored: publicArmored(t, "B", "b@johal.in")},
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	}))
	defer srv.Close()

	got, err := pgp.FetchKeyDirectory(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d keys, want 2", len(got))
	}
}

func TestKeyDirectoryRejectsPlainHTTP(t *testing.T) {
	if _, err := pgp.DiscoverKeyDirectory(context.Background(), http.DefaultClient,
		"http://insecure.example/keys", "x@y.z"); err == nil ||
		!strings.Contains(err.Error(), "https") {
		t.Fatalf("expected https rejection, got %v", err)
	}
}
