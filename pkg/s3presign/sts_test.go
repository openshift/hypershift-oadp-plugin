package s3presign

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssumeRoleWithWebIdentity(t *testing.T) {
	t.Run("successful response returns credentials", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}

			if err := r.ParseForm(); err != nil {
				t.Fatalf("failed to parse form: %v", err)
			}

			if r.FormValue("Action") != "AssumeRoleWithWebIdentity" {
				t.Errorf("unexpected Action: %q", r.FormValue("Action"))
			}
			if r.FormValue("RoleArn") != "arn:aws:iam::123456789012:role/test" {
				t.Errorf("unexpected RoleArn: %q", r.FormValue("RoleArn"))
			}
			if r.FormValue("RoleSessionName") != "test-session" {
				t.Errorf("unexpected RoleSessionName: %q", r.FormValue("RoleSessionName"))
			}
			if r.FormValue("WebIdentityToken") != "my-token-content" {
				t.Errorf("unexpected WebIdentityToken: %q", r.FormValue("WebIdentityToken"))
			}

			w.Header().Set("Content-Type", "text/xml")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<AssumeRoleWithWebIdentityResponse>
  <AssumeRoleWithWebIdentityResult>
    <Credentials>
      <AccessKeyId>ASIATESTACCESSKEY</AccessKeyId>
      <SecretAccessKey>testSecretKey123</SecretAccessKey>
      <SessionToken>testSessionToken456</SessionToken>
    </Credentials>
  </AssumeRoleWithWebIdentityResult>
</AssumeRoleWithWebIdentityResponse>`))
		}))
		defer server.Close()

		tokenFile := writeTokenFile(t, "my-token-content")

		client := &STSClient{
			HTTPClient: server.Client(),
			Endpoint:   server.URL,
		}

		creds, err := client.AssumeRoleWithWebIdentity(
			context.Background(),
			"arn:aws:iam::123456789012:role/test",
			tokenFile,
			"test-session",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if creds.AccessKeyID != "ASIATESTACCESSKEY" {
			t.Errorf("unexpected AccessKeyID: %q", creds.AccessKeyID)
		}
		if creds.SecretAccessKey != "testSecretKey123" {
			t.Errorf("unexpected SecretAccessKey: %q", creds.SecretAccessKey)
		}
		if creds.SessionToken != "testSessionToken456" {
			t.Errorf("unexpected SessionToken: %q", creds.SessionToken)
		}
	})

	t.Run("STS returns HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`<ErrorResponse><Error><Code>AccessDenied</Code><Message>Not authorized</Message></Error></ErrorResponse>`))
		}))
		defer server.Close()

		tokenFile := writeTokenFile(t, "my-token")

		client := &STSClient{
			HTTPClient: server.Client(),
			Endpoint:   server.URL,
		}

		_, err := client.AssumeRoleWithWebIdentity(
			context.Background(),
			"arn:aws:iam::123456789012:role/test",
			tokenFile,
			"test-session",
		)
		if err == nil {
			t.Fatal("expected error for HTTP 403")
		}
		if !strings.Contains(err.Error(), "403") {
			t.Errorf("error should mention HTTP status code, got: %v", err)
		}
	})

	t.Run("malformed XML response returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`not xml at all`))
		}))
		defer server.Close()

		tokenFile := writeTokenFile(t, "my-token")

		client := &STSClient{
			HTTPClient: server.Client(),
			Endpoint:   server.URL,
		}

		_, err := client.AssumeRoleWithWebIdentity(
			context.Background(),
			"arn:aws:iam::123456789012:role/test",
			tokenFile,
			"test-session",
		)
		if err == nil {
			t.Fatal("expected error for malformed XML")
		}
	})

	missingCredTests := []struct {
		name           string
		accessKeyID    string
		secretAccessKey string
		sessionToken   string
	}{
		{
			name:            "Given all credentials empty, When STS responds, Then it should return error",
			accessKeyID:     "",
			secretAccessKey: "",
			sessionToken:    "",
		},
		{
			name:            "Given valid keys but empty session token, When STS responds, Then it should return error",
			accessKeyID:     "ASIATESTACCESSKEY",
			secretAccessKey: "testSecretKey123",
			sessionToken:    "",
		},
		{
			name:            "Given valid session token but empty access key, When STS responds, Then it should return error",
			accessKeyID:     "",
			secretAccessKey: "testSecretKey123",
			sessionToken:    "testSessionToken456",
		},
		{
			name:            "Given valid session token but empty secret key, When STS responds, Then it should return error",
			accessKeyID:     "ASIATESTACCESSKEY",
			secretAccessKey: "",
			sessionToken:    "testSessionToken456",
		},
	}

	for _, tt := range missingCredTests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(buildSTSCredentialResponse(tt.accessKeyID, tt.secretAccessKey, tt.sessionToken)))
			}))
			defer server.Close()

			tokenFile := writeTokenFile(t, "my-token")

			client := &STSClient{
				HTTPClient: server.Client(),
				Endpoint:   server.URL,
			}

			_, err := client.AssumeRoleWithWebIdentity(
				context.Background(),
				"arn:aws:iam::123456789012:role/test",
				tokenFile,
				"test-session",
			)
			if err == nil {
				t.Fatal("expected error for missing credentials")
			}
		})
	}

	t.Run("missing token file returns error", func(t *testing.T) {
		client := NewSTSClient()
		_, err := client.AssumeRoleWithWebIdentity(
			context.Background(),
			"arn:aws:iam::123456789012:role/test",
			"/nonexistent/path/token",
			"test-session",
		)
		if err == nil {
			t.Fatal("expected error for missing token file")
		}
	})

	t.Run("empty token file returns error", func(t *testing.T) {
		tokenFile := writeTokenFile(t, "")

		client := NewSTSClient()
		_, err := client.AssumeRoleWithWebIdentity(
			context.Background(),
			"arn:aws:iam::123456789012:role/test",
			tokenFile,
			"test-session",
		)
		if err == nil {
			t.Fatal("expected error for empty token file")
		}
	})

	t.Run("empty roleARN returns error", func(t *testing.T) {
		client := NewSTSClient()
		_, err := client.AssumeRoleWithWebIdentity(context.Background(), "", "/some/token", "session")
		if err == nil {
			t.Fatal("expected error for empty roleARN")
		}
	})

	t.Run("empty tokenFile path returns error", func(t *testing.T) {
		client := NewSTSClient()
		_, err := client.AssumeRoleWithWebIdentity(context.Background(), "arn:aws:iam::123:role/r", "", "session")
		if err == nil {
			t.Fatal("expected error for empty tokenFile")
		}
	})

	t.Run("default session name is used when empty", func(t *testing.T) {
		var receivedSessionName string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.ParseForm()
			receivedSessionName = r.FormValue("RoleSessionName")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<AssumeRoleWithWebIdentityResponse>
  <AssumeRoleWithWebIdentityResult>
    <Credentials>
      <AccessKeyId>ASIA123</AccessKeyId>
      <SecretAccessKey>secret</SecretAccessKey>
      <SessionToken>token</SessionToken>
    </Credentials>
  </AssumeRoleWithWebIdentityResult>
</AssumeRoleWithWebIdentityResponse>`))
		}))
		defer server.Close()

		tokenFile := writeTokenFile(t, "tok")
		client := &STSClient{
			HTTPClient: server.Client(),
			Endpoint:   server.URL,
		}

		_, err := client.AssumeRoleWithWebIdentity(
			context.Background(),
			"arn:aws:iam::123456789012:role/test",
			tokenFile,
			"",
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if receivedSessionName != defaultSessionName {
			t.Errorf("expected default session name %q, got %q", defaultSessionName, receivedSessionName)
		}
	})
}

func TestNewSTSClient(t *testing.T) {
	client := NewSTSClient()
	if client.Endpoint != defaultSTSEndpoint {
		t.Errorf("expected endpoint %q, got %q", defaultSTSEndpoint, client.Endpoint)
	}
	if client.HTTPClient == nil {
		t.Error("HTTPClient should not be nil")
	}
}

func buildSTSCredentialResponse(accessKeyID, secretAccessKey, sessionToken string) string {
	return fmt.Sprintf(`<AssumeRoleWithWebIdentityResponse>
  <AssumeRoleWithWebIdentityResult>
    <Credentials>
      <AccessKeyId>%s</AccessKeyId>
      <SecretAccessKey>%s</SecretAccessKey>
      <SessionToken>%s</SessionToken>
    </Credentials>
  </AssumeRoleWithWebIdentityResult>
</AssumeRoleWithWebIdentityResponse>`, accessKeyID, secretAccessKey, sessionToken)
}

// writeTokenFile creates a temporary file with the given content and returns its path.
func writeTokenFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write token file: %v", err)
	}
	return path
}
