package azblobsas

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGetUserDelegationKey(t *testing.T) {
	fixedTime := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	validResponse := `<?xml version="1.0" encoding="utf-8"?>
<UserDelegationKey>
    <SignedOid>oid-1234</SignedOid>
    <SignedTid>tid-5678</SignedTid>
    <SignedStart>2026-05-08T12:00:00Z</SignedStart>
    <SignedExpiry>2026-05-08T13:00:00Z</SignedExpiry>
    <SignedService>b</SignedService>
    <SignedVersion>2024-11-04</SignedVersion>
    <Value>dGVzdC1kZWxlZ2F0aW9uLWtleS12YWx1ZSEh</Value>
</UserDelegationKey>`

	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantErr    bool
		errContain string
		assert     func(*testing.T, *UserDelegationKey)
	}{
		{
			name: "successful delegation key response",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if !strings.Contains(r.URL.RawQuery, "restype=service") {
					t.Error("missing restype=service query param")
				}
				if !strings.Contains(r.URL.RawQuery, "comp=userdelegationkey") {
					t.Error("missing comp=userdelegationkey query param")
				}
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
				}
				if r.Header.Get("x-ms-version") != sasVersion {
					t.Errorf("unexpected x-ms-version: %s", r.Header.Get("x-ms-version"))
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(validResponse))
			},
			assert: func(t *testing.T, key *UserDelegationKey) {
				if key.SignedOID != "oid-1234" {
					t.Errorf("SignedOID = %q, want %q", key.SignedOID, "oid-1234")
				}
				if key.SignedTID != "tid-5678" {
					t.Errorf("SignedTID = %q, want %q", key.SignedTID, "tid-5678")
				}
				if key.SignedStart != "2026-05-08T12:00:00Z" {
					t.Errorf("SignedStart = %q", key.SignedStart)
				}
				if key.SignedExpiry != "2026-05-08T13:00:00Z" {
					t.Errorf("SignedExpiry = %q", key.SignedExpiry)
				}
				if key.SignedService != "b" {
					t.Errorf("SignedService = %q", key.SignedService)
				}
				if key.SignedVersion != "2024-11-04" {
					t.Errorf("SignedVersion = %q", key.SignedVersion)
				}
				if key.Value != "dGVzdC1kZWxlZ2F0aW9uLWtleS12YWx1ZSEh" {
					t.Errorf("Value = %q", key.Value)
				}
			},
		},
		{
			name: "403 forbidden",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte("AuthorizationFailure"))
			},
			wantErr:    true,
			errContain: "403",
		},
		{
			name: "500 server error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("InternalError"))
			},
			wantErr:    true,
			errContain: "500",
		},
		{
			name: "malformed XML response",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("not xml at all"))
			},
			wantErr:    true,
			errContain: "parsing delegation key response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			origHTTPDo := httpDo
			httpDo = http.DefaultClient.Do
			defer func() { httpDo = origHTTPDo }()

			key, err := GetUserDelegationKey(
				context.Background(),
				"myaccount",
				"test-token",
				fixedTime,
				fixedTime.Add(1*time.Hour),
				server.URL,
			)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.assert != nil {
				tt.assert(t, key)
			}
		})
	}
}

func TestGenerateUserDelegationSASBlobURL(t *testing.T) {
	fixedTime := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	origNow := nowFunc
	nowFunc = func() time.Time { return fixedTime }
	defer func() { nowFunc = origNow }()

	testKey := &UserDelegationKey{
		SignedOID:     "oid-1234",
		SignedTID:     "tid-5678",
		SignedStart:   "2026-05-08T12:00:00Z",
		SignedExpiry:  "2026-05-08T13:00:00Z",
		SignedService: "b",
		SignedVersion: "2024-11-04",
		Value:         base64.StdEncoding.EncodeToString([]byte("test-delegation-key-value!!!!")),
	}

	baseOpts := UserDelegationSASOptions{
		Account:       "myaccount",
		Container:     "mycontainer",
		Blob:          "path/to/snapshot.db",
		DelegationKey: testKey,
		Expiry:        1 * time.Hour,
	}

	t.Run("basic URL structure with all delegation params", func(t *testing.T) {
		result, err := GenerateUserDelegationSASBlobURL(baseOpts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.HasPrefix(result, "https://") {
			t.Errorf("URL should start with https://, got %s", result)
		}

		parsed, err := url.Parse(result)
		if err != nil {
			t.Fatalf("failed to parse result URL: %v", err)
		}

		if parsed.Host != "myaccount.blob.core.windows.net" {
			t.Errorf("unexpected host: %s", parsed.Host)
		}

		expectedPath := "/mycontainer/path/to/snapshot.db"
		if parsed.Path != expectedPath {
			t.Errorf("unexpected path: got %s, want %s", parsed.Path, expectedPath)
		}

		standardParams := []string{"sv", "st", "se", "sr", "sp", "spr", "sig"}
		for _, p := range standardParams {
			if parsed.Query().Get(p) == "" {
				t.Errorf("missing required query param %s", p)
			}
		}

		delegationParams := map[string]string{
			"skoid": "oid-1234",
			"sktid": "tid-5678",
			"skt":   "2026-05-08T12:00:00Z",
			"ske":   "2026-05-08T13:00:00Z",
			"sks":   "b",
			"skv":   "2024-11-04",
		}
		for param, expected := range delegationParams {
			got := parsed.Query().Get(param)
			if got != expected {
				t.Errorf("param %s = %q, want %q", param, got, expected)
			}
		}

		if parsed.Query().Get("sv") != sasVersion {
			t.Errorf("unexpected version: %s", parsed.Query().Get("sv"))
		}
		if parsed.Query().Get("sr") != "b" {
			t.Errorf("unexpected resource: %s", parsed.Query().Get("sr"))
		}
		if parsed.Query().Get("sp") != "r" {
			t.Errorf("unexpected permission: %s", parsed.Query().Get("sp"))
		}
		if parsed.Query().Get("se") != "2026-05-08T13:00:00Z" {
			t.Errorf("unexpected expiry: %s", parsed.Query().Get("se"))
		}
	})

	t.Run("deterministic output", func(t *testing.T) {
		result1, _ := GenerateUserDelegationSASBlobURL(baseOpts)
		result2, _ := GenerateUserDelegationSASBlobURL(baseOpts)
		if result1 != result2 {
			t.Errorf("same inputs should produce same output")
		}
	})

	t.Run("custom endpoint", func(t *testing.T) {
		opts := baseOpts
		opts.Endpoint = "https://azurite.example.com:10000"

		result, err := GenerateUserDelegationSASBlobURL(opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		parsed, _ := url.Parse(result)
		if parsed.Host != "azurite.example.com:10000" {
			t.Errorf("expected custom endpoint host, got %s", parsed.Host)
		}
	})

	t.Run("http endpoint", func(t *testing.T) {
		opts := baseOpts
		opts.Endpoint = "http://127.0.0.1:10000"

		result, err := GenerateUserDelegationSASBlobURL(opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.HasPrefix(result, "http://") {
			t.Errorf("URL should start with http:// for http endpoint, got %s", result)
		}
	})

	t.Run("missing required fields", func(t *testing.T) {
		_, err := GenerateUserDelegationSASBlobURL(UserDelegationSASOptions{})
		if err == nil {
			t.Error("expected error for empty options")
		}
	})

	t.Run("nil delegation key", func(t *testing.T) {
		opts := baseOpts
		opts.DelegationKey = nil
		_, err := GenerateUserDelegationSASBlobURL(opts)
		if err == nil {
			t.Error("expected error for nil delegation key")
		}
	})

	t.Run("different blobs produce different signatures", func(t *testing.T) {
		opts1 := baseOpts
		opts1.Blob = "blob-a"

		opts2 := baseOpts
		opts2.Blob = "blob-b"

		result1, _ := GenerateUserDelegationSASBlobURL(opts1)
		result2, _ := GenerateUserDelegationSASBlobURL(opts2)

		parsed1, _ := url.Parse(result1)
		parsed2, _ := url.Parse(result2)

		if parsed1.Query().Get("sig") == parsed2.Query().Get("sig") {
			t.Error("different blobs should produce different signatures")
		}
	})

	t.Run("different delegation keys produce different signatures", func(t *testing.T) {
		key2 := &UserDelegationKey{
			SignedOID:     "oid-other",
			SignedTID:     "tid-other",
			SignedStart:   "2026-05-08T12:00:00Z",
			SignedExpiry:  "2026-05-08T13:00:00Z",
			SignedService: "b",
			SignedVersion: "2024-11-04",
			Value:         base64.StdEncoding.EncodeToString([]byte("different-delegation-key-val!")),
		}

		opts1 := baseOpts
		opts2 := baseOpts
		opts2.DelegationKey = key2

		result1, _ := GenerateUserDelegationSASBlobURL(opts1)
		result2, _ := GenerateUserDelegationSASBlobURL(opts2)

		parsed1, _ := url.Parse(result1)
		parsed2, _ := url.Parse(result2)

		if parsed1.Query().Get("sig") == parsed2.Query().Get("sig") {
			t.Error("different delegation keys should produce different signatures")
		}
	})

	t.Run("default expiry when zero", func(t *testing.T) {
		opts := baseOpts
		opts.Expiry = 0

		result, err := GenerateUserDelegationSASBlobURL(opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		parsed, _ := url.Parse(result)
		if parsed.Query().Get("se") != "2026-05-08T13:00:00Z" {
			t.Errorf("expected default 1h expiry, got se=%s", parsed.Query().Get("se"))
		}
	})
}
