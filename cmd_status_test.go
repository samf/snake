package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func strPtr(s string) *string { return &s }

func TestFormatIdent(t *testing.T) {
	tests := []struct {
		name  string
		me    meResponse
		want  string
	}{
		{"name and email", meResponse{Name: strPtr("Alice"), Email: strPtr("alice@example.com")}, "Alice <alice@example.com>"},
		{"email only", meResponse{Email: strPtr("alice@example.com")}, "alice@example.com"},
		{"name only", meResponse{Name: strPtr("Alice")}, "Alice"},
		{"neither", meResponse{}, "logged in"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatIdent(&tt.me); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFetchMe_OK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/me" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing or wrong Authorization header")
		}
		json.NewEncoder(w).Encode(meResponse{Name: strPtr("Alice"), Email: strPtr("alice@example.com")})
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "test-token"}
	me, err := fetchMe(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if me == nil {
		t.Fatal("expected non-nil meResponse")
	}
	if me.Name == nil || *me.Name != "Alice" {
		t.Errorf("unexpected name: %v", me.Name)
	}
}

func TestFetchMe_Unauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "bad-token"}
	me, err := fetchMe(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if me != nil {
		t.Errorf("expected nil for 401, got %+v", me)
	}
}

func TestFetchMe_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	cfg := &Config{Server: ts.URL, Token: "tok"}
	_, err := fetchMe(cfg)
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}
