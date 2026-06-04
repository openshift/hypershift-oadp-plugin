package azblobsas

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParseAzBlobURL(t *testing.T) {
	tests := []struct {
		name          string
		rawURL        string
		wantAccount   string
		wantContainer string
		wantBlob      string
		wantErr       bool
	}{
		{
			name:          "valid public cloud URL",
			rawURL:        "https://myaccount.blob.core.windows.net/mycontainer/path/to/snapshot.db",
			wantAccount:   "myaccount",
			wantContainer: "mycontainer",
			wantBlob:      "path/to/snapshot.db",
		},
		{
			name:          "valid URL with simple blob name",
			rawURL:        "https://sa1.blob.core.windows.net/backups/snapshot.db",
			wantAccount:   "sa1",
			wantContainer: "backups",
			wantBlob:      "snapshot.db",
		},
		{
			name:          "government cloud URL",
			rawURL:        "https://myaccount.blob.core.usgovcloudapi.net/container/blob",
			wantAccount:   "myaccount",
			wantContainer: "container",
			wantBlob:      "blob",
		},
		{
			name:          "china cloud URL",
			rawURL:        "https://myaccount.blob.core.chinacloudapi.cn/container/blob",
			wantAccount:   "myaccount",
			wantContainer: "container",
			wantBlob:      "blob",
		},
		{
			name:    "missing blob path",
			rawURL:  "https://myaccount.blob.core.windows.net/container",
			wantErr: true,
		},
		{
			name:    "missing container and blob",
			rawURL:  "https://myaccount.blob.core.windows.net/",
			wantErr: true,
		},
		{
			name:    "wrong scheme",
			rawURL:  "s3://bucket/key",
			wantErr: true,
		},
		{
			name:    "not a blob URL",
			rawURL:  "https://example.com/container/blob",
			wantErr: true,
		},
		{
			name:    "empty URL",
			rawURL:  "",
			wantErr: true,
		},
		{
			name:    "trailing slash only",
			rawURL:  "https://myaccount.blob.core.windows.net/container/",
			wantErr: true,
		},
		{
			name:          "http scheme",
			rawURL:        "http://devaccount.blob.core.windows.net/container/blob",
			wantAccount:   "devaccount",
			wantContainer: "container",
			wantBlob:      "blob",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account, container, blob, err := ParseAzBlobURL(tt.rawURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAzBlobURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if account != tt.wantAccount {
				t.Errorf("ParseAzBlobURL() account = %q, want %q", account, tt.wantAccount)
			}
			if container != tt.wantContainer {
				t.Errorf("ParseAzBlobURL() container = %q, want %q", container, tt.wantContainer)
			}
			if blob != tt.wantBlob {
				t.Errorf("ParseAzBlobURL() blob = %q, want %q", blob, tt.wantBlob)
			}
		})
	}
}

func TestIsAzBlobURL(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		want   bool
	}{
		{"azure public cloud", "https://myaccount.blob.core.windows.net/container/blob", true},
		{"azure gov cloud", "https://myaccount.blob.core.usgovcloudapi.net/container/blob", true},
		{"azure china cloud", "https://myaccount.blob.core.chinacloudapi.cn/container/blob", true},
		{"http scheme", "http://devaccount.blob.core.windows.net/container/blob", true},
		{"container only no blob", "https://myaccount.blob.core.windows.net/container", true},
		{"s3 scheme", "s3://bucket/key", false},
		{"s3 https", "https://my-bucket.s3.us-east-1.amazonaws.com/path/to/snapshot.db", false},
		{"non-azure https", "https://example.com/container/blob", false},
		{"empty string", "", false},
		{"not a url", "not-a-url", false},
		{"ftp scheme", "ftp://myaccount.blob.core.windows.net/c/b", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAzBlobURL(tt.rawURL); got != tt.want {
				t.Errorf("IsAzBlobURL(%q) = %v, want %v", tt.rawURL, got, tt.want)
			}
		})
	}
}

func TestParseAzureCredentials(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		keyEnvVar string
		wantKey   string
		wantErr   bool
	}{
		{
			name: "standard storage account key",
			data: `AZURE_STORAGE_ACCOUNT_ACCESS_KEY=dGVzdGtleQ==
AZURE_CLOUD_NAME=AzurePublicCloud
`,
			keyEnvVar: "",
			wantKey:   "dGVzdGtleQ==",
		},
		{
			name: "custom key env var name",
			data: `MY_CUSTOM_KEY=customvalue
AZURE_CLOUD_NAME=AzurePublicCloud
`,
			keyEnvVar: "MY_CUSTOM_KEY",
			wantKey:   "customvalue",
		},
		{
			name: "comments and blank lines",
			data: `# This is a comment
AZURE_CLOUD_NAME=AzurePublicCloud

AZURE_STORAGE_ACCOUNT_ACCESS_KEY=thekey
`,
			keyEnvVar: "",
			wantKey:   "thekey",
		},
		{
			name: "whitespace around equals",
			data: `AZURE_STORAGE_ACCOUNT_ACCESS_KEY = spacedvalue
`,
			keyEnvVar: "",
			wantKey:   "spacedvalue",
		},
		{
			name:      "missing key",
			data:      "AZURE_CLOUD_NAME=AzurePublicCloud\n",
			keyEnvVar: "",
			wantErr:   true,
		},
		{
			name:      "empty data",
			data:      "",
			keyEnvVar: "",
			wantErr:   true,
		},
		{
			name: "value with equals sign",
			data: `AZURE_STORAGE_ACCOUNT_ACCESS_KEY=abc=def==
`,
			keyEnvVar: "",
			wantKey:   "abc=def==",
		},
		{
			name: "multiple entries picks correct one",
			data: `AZURE_SUBSCRIPTION_ID=sub123
AZURE_TENANT_ID=tenant456
AZURE_STORAGE_ACCOUNT_ACCESS_KEY=correctkey
AZURE_RESOURCE_GROUP=rg1
`,
			keyEnvVar: "",
			wantKey:   "correctkey",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds, err := ParseAzureCredentials([]byte(tt.data), tt.keyEnvVar)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAzureCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if creds.StorageAccountAccessKey != tt.wantKey {
				t.Errorf("StorageAccountAccessKey = %q, want %q", creds.StorageAccountAccessKey, tt.wantKey)
			}
		})
	}
}

func TestParseAzureAADCredentials(t *testing.T) {
	tests := []struct {
		name         string
		data         string
		wantTenantID string
		wantClientID string
		wantSecret   string
		wantCloud    string
		wantErr      bool
	}{
		{
			name: "workload identity credentials",
			data: `AZURE_SUBSCRIPTION_ID=sub123
AZURE_TENANT_ID=tenant456
AZURE_CLIENT_ID=client789
AZURE_RESOURCE_GROUP=rg1
AZURE_CLOUD_NAME=AzurePublicCloud
`,
			wantTenantID: "tenant456",
			wantClientID: "client789",
			wantCloud:    "AzurePublicCloud",
		},
		{
			name: "service principal with client secret",
			data: `AZURE_TENANT_ID=t1
AZURE_CLIENT_ID=c1
AZURE_CLIENT_SECRET=s1
AZURE_CLOUD_NAME=AzureUSGovernmentCloud
`,
			wantTenantID: "t1",
			wantClientID: "c1",
			wantSecret:   "s1",
			wantCloud:    "AzureUSGovernmentCloud",
		},
		{
			name: "comments and blank lines",
			data: `# Azure credentials
AZURE_TENANT_ID=t2

AZURE_CLIENT_ID=c2
`,
			wantTenantID: "t2",
			wantClientID: "c2",
		},
		{
			name:    "missing tenant ID",
			data:    "AZURE_CLIENT_ID=c1\n",
			wantErr: true,
		},
		{
			name:    "missing client ID",
			data:    "AZURE_TENANT_ID=t1\n",
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds, err := ParseAzureAADCredentials([]byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAzureAADCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if creds.TenantID != tt.wantTenantID {
				t.Errorf("TenantID = %q, want %q", creds.TenantID, tt.wantTenantID)
			}
			if creds.ClientID != tt.wantClientID {
				t.Errorf("ClientID = %q, want %q", creds.ClientID, tt.wantClientID)
			}
			if creds.ClientSecret != tt.wantSecret {
				t.Errorf("ClientSecret = %q, want %q", creds.ClientSecret, tt.wantSecret)
			}
			if creds.CloudName != tt.wantCloud {
				t.Errorf("CloudName = %q, want %q", creds.CloudName, tt.wantCloud)
			}
		})
	}
}

func TestGenerateSASBlobURL(t *testing.T) {
	fixedTime := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	origNow := nowFunc
	nowFunc = func() time.Time { return fixedTime }
	defer func() { nowFunc = origNow }()

	testKey := base64.StdEncoding.EncodeToString([]byte("test-storage-account-key-value!!"))

	baseOpts := SASOptions{
		Account:    "myaccount",
		Container:  "mycontainer",
		Blob:       "path/to/snapshot.db",
		AccountKey: testKey,
		Expiry:     1 * time.Hour,
	}

	t.Run("basic SAS URL structure", func(t *testing.T) {
		result, err := GenerateSASBlobURL(baseOpts)
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

		requiredParams := []string{"sv", "st", "se", "sr", "sp", "spr", "sig"}
		for _, p := range requiredParams {
			if parsed.Query().Get(p) == "" {
				t.Errorf("missing required query param %s", p)
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
		if parsed.Query().Get("spr") != "https" {
			t.Errorf("unexpected protocol: %s", parsed.Query().Get("spr"))
		}
		if parsed.Query().Get("se") != "2026-05-08T13:00:00Z" {
			t.Errorf("unexpected expiry: %s", parsed.Query().Get("se"))
		}
		if parsed.Query().Get("st") != "2026-05-08T12:00:00Z" {
			t.Errorf("unexpected start: %s", parsed.Query().Get("st"))
		}
	})

	t.Run("deterministic output", func(t *testing.T) {
		result1, _ := GenerateSASBlobURL(baseOpts)
		result2, _ := GenerateSASBlobURL(baseOpts)
		if result1 != result2 {
			t.Errorf("same inputs should produce same output")
		}
	})

	t.Run("custom endpoint", func(t *testing.T) {
		opts := baseOpts
		opts.Endpoint = "https://azurite.example.com:10000"

		result, err := GenerateSASBlobURL(opts)
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

		result, err := GenerateSASBlobURL(opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.HasPrefix(result, "http://") {
			t.Errorf("URL should start with http:// for http endpoint, got %s", result)
		}
	})

	t.Run("default expiry when zero", func(t *testing.T) {
		opts := baseOpts
		opts.Expiry = 0

		result, err := GenerateSASBlobURL(opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		parsed, _ := url.Parse(result)
		if parsed.Query().Get("se") != "2026-05-08T13:00:00Z" {
			t.Errorf("expected default 1h expiry, got se=%s", parsed.Query().Get("se"))
		}
	})

	t.Run("missing required fields", func(t *testing.T) {
		_, err := GenerateSASBlobURL(SASOptions{})
		if err == nil {
			t.Error("expected error for empty options")
		}
	})

	t.Run("missing account key", func(t *testing.T) {
		opts := baseOpts
		opts.AccountKey = ""

		_, err := GenerateSASBlobURL(opts)
		if err == nil {
			t.Error("expected error for missing account key")
		}
	})

	t.Run("invalid base64 account key", func(t *testing.T) {
		opts := baseOpts
		opts.AccountKey = "not-valid-base64!!!"

		_, err := GenerateSASBlobURL(opts)
		if err == nil {
			t.Error("expected error for invalid base64 key")
		}
	})

	t.Run("special characters in blob path", func(t *testing.T) {
		opts := baseOpts
		opts.Blob = "path/to/snap shot#1.db"

		result, err := GenerateSASBlobURL(opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		parsed, err := url.Parse(result)
		if err != nil {
			t.Fatalf("failed to parse SAS URL: %v", err)
		}
		if parsed.Query().Get("sig") == "" {
			t.Error("missing sig parameter")
		}
	})

	t.Run("signature changes with different blob", func(t *testing.T) {
		opts1 := baseOpts
		opts1.Blob = "blob-a"

		opts2 := baseOpts
		opts2.Blob = "blob-b"

		result1, _ := GenerateSASBlobURL(opts1)
		result2, _ := GenerateSASBlobURL(opts2)

		parsed1, _ := url.Parse(result1)
		parsed2, _ := url.Parse(result2)

		if parsed1.Query().Get("sig") == parsed2.Query().Get("sig") {
			t.Error("different blobs should produce different signatures")
		}
	})

	t.Run("signature changes with different key", func(t *testing.T) {
		opts1 := baseOpts
		opts2 := baseOpts
		opts2.AccountKey = base64.StdEncoding.EncodeToString([]byte("different-key-value-here-longer!"))

		result1, _ := GenerateSASBlobURL(opts1)
		result2, _ := GenerateSASBlobURL(opts2)

		parsed1, _ := url.Parse(result1)
		parsed2, _ := url.Parse(result2)

		if parsed1.Query().Get("sig") == parsed2.Query().Get("sig") {
			t.Error("different keys should produce different signatures")
		}
	})
}
