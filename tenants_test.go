package main

import (
	"net/http/httptest"
	"os"
	"testing"
)

func TestTenantsIsolateUsers(t *testing.T) {
	dir := t.TempDir()
	tn := newTenants(dir, nil, "relay.example")

	alice1, err := tn.server("alice")
	if err != nil {
		t.Fatal(err)
	}
	alice2, err := tn.server("alice")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := tn.server("bob")
	if err != nil {
		t.Fatal(err)
	}

	if alice1 != alice2 {
		t.Error("same user must reuse one server")
	}
	if alice1 == bob {
		t.Error("different users must not share a server")
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("want one identity file per user, got %d", len(files))
	}
}

func TestBearerUser(t *testing.T) {
	r := httptest.NewRequest("POST", "/mcp", nil)
	if _, ok := bearerUser(r); ok {
		t.Error("a request with no bearer token must be rejected")
	}
	r.Header.Set("Authorization", "Bearer tok-123")
	if u, ok := bearerUser(r); !ok || u != "tok-123" {
		t.Fatalf("got user=%q ok=%v, want tok-123,true", u, ok)
	}
}
