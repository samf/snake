package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPollToken(t *testing.T) {
	tests := []struct {
		name        string
		response    tokenPollResponse
		statusCode  int
		wantToken   string
		wantPending bool
		wantErr     bool
	}{
		{
			name:        "authorization pending",
			response:    tokenPollResponse{Error: "authorization_pending"},
			wantPending: true,
		},
		{
			name:      "success",
			response:  tokenPollResponse{Token: "my-token"},
			wantToken: "my-token",
		},
		{
			name:    "expired token",
			response: tokenPollResponse{Error: "expired_token"},
			wantErr: true,
		},
		{
			name:    "access denied",
			response: tokenPollResponse{Error: "access_denied"},
			wantErr: true,
		},
		{
			name:        "unknown error treated as pending",
			response:    tokenPollResponse{Error: "slow_down"},
			wantPending: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(tt.response)
			}))
			defer ts.Close()

			token, pending, err := pollToken(ts.URL, "device-code-123")
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pending != tt.wantPending {
				t.Errorf("pending: got %v, want %v", pending, tt.wantPending)
			}
			if token != tt.wantToken {
				t.Errorf("token: got %q, want %q", token, tt.wantToken)
			}
		})
	}
}

func TestResolveCanID(t *testing.T) {
	tests := []struct {
		name     string
		response cansAPIResponse
		wantID   string
		wantName string
		wantErr  bool
	}{
		{
			name:    "no cans",
			response: cansAPIResponse{},
			wantErr: true,
		},
		{
			name: "single can",
			response: cansAPIResponse{
				Cans: []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{{ID: "can-1", Name: "My Can"}},
			},
			wantID:   "can-1",
			wantName: "My Can",
		},
		{
			name: "single can without name uses untitled",
			response: cansAPIResponse{
				Cans: []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{{ID: "can-1", Name: ""}},
			},
			wantID:   "can-1",
			wantName: "untitled",
		},
		{
			name: "multiple cans with preferred",
			response: cansAPIResponse{
				Cans: []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{{ID: "can-1", Name: "First"}, {ID: "can-2", Name: "Second"}},
				PreferredCan: "can-2",
			},
			wantID:   "can-2",
			wantName: "Second",
		},
		{
			name: "multiple cans without preferred",
			response: cansAPIResponse{
				Cans: []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}{{ID: "can-1", Name: "First"}, {ID: "can-2", Name: "Second"}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("missing or wrong Authorization header")
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tt.response)
			}))
			defer ts.Close()

			id, name, err := resolveCanID(ts.URL, "test-token")
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tt.wantID {
				t.Errorf("id: got %q, want %q", id, tt.wantID)
			}
			if name != tt.wantName {
				t.Errorf("name: got %q, want %q", name, tt.wantName)
			}
		})
	}
}
