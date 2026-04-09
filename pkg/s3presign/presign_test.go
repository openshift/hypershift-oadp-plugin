package s3presign

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParseS3URL(t *testing.T) {
	tests := []struct {
		name       string
		rawURL     string
		wantBucket string
		wantKey    string
		wantErr    bool
	}{
		{
			name:       "valid URL with nested key",
			rawURL:     "s3://my-bucket/path/to/snapshot.db",
			wantBucket: "my-bucket",
			wantKey:    "path/to/snapshot.db",
		},
		{
			name:       "valid URL with simple key",
			rawURL:     "s3://bucket/key",
			wantBucket: "bucket",
			wantKey:    "key",
		},
		{
			name:    "missing key - bucket only",
			rawURL:  "s3://my-bucket",
			wantErr: true,
		},
		{
			name:    "missing key - trailing slash",
			rawURL:  "s3://my-bucket/",
			wantErr: true,
		},
		{
			name:    "wrong scheme",
			rawURL:  "https://bucket/key",
			wantErr: true,
		},
		{
			name:    "empty URL",
			rawURL:  "",
			wantErr: true,
		},
		{
			name:       "special characters in key",
			rawURL:     "s3://bucket/path/to/snap%20shot.db",
			wantBucket: "bucket",
			wantKey:    "path/to/snap shot.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, key, err := ParseS3URL(tt.rawURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseS3URL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if bucket != tt.wantBucket {
				t.Errorf("ParseS3URL() bucket = %q, want %q", bucket, tt.wantBucket)
			}
			if key != tt.wantKey {
				t.Errorf("ParseS3URL() key = %q, want %q", key, tt.wantKey)
			}
		})
	}
}

func TestParseAWSCredentials(t *testing.T) {
	tests := []struct {
		name           string
		data           string
		profile        string
		wantAccessKey  string
		wantSecretKey  string
		wantToken      string
		wantErr        bool
	}{
		{
			name: "standard default profile",
			data: `[default]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
`,
			profile:       "default",
			wantAccessKey: "AKIAIOSFODNN7EXAMPLE",
			wantSecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		},
		{
			name: "profile with session token",
			data: `[default]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
aws_session_token = FwoGZXIvYXdzEBYaDHqa0AP
`,
			profile:       "default",
			wantAccessKey: "AKIAIOSFODNN7EXAMPLE",
			wantSecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			wantToken:     "FwoGZXIvYXdzEBYaDHqa0AP",
		},
		{
			name: "multiple profiles - select non-default",
			data: `[default]
aws_access_key_id = DEFAULT_KEY
aws_secret_access_key = DEFAULT_SECRET

[production]
aws_access_key_id = PROD_KEY
aws_secret_access_key = PROD_SECRET
`,
			profile:       "production",
			wantAccessKey: "PROD_KEY",
			wantSecretKey: "PROD_SECRET",
		},
		{
			name: "comments and blank lines",
			data: `# This is a comment
; This is also a comment

[default]
  aws_access_key_id  =  AKID
  aws_secret_access_key  =  SECRET
`,
			profile:       "default",
			wantAccessKey: "AKID",
			wantSecretKey: "SECRET",
		},
		{
			name: "missing profile",
			data: `[default]
aws_access_key_id = AKID
aws_secret_access_key = SECRET
`,
			profile: "nonexistent",
			wantErr: true,
		},
		{
			name: "missing access key",
			data: `[default]
aws_secret_access_key = SECRET
`,
			profile: "default",
			wantErr: true,
		},
		{
			name: "missing secret key",
			data: `[default]
aws_access_key_id = AKID
`,
			profile: "default",
			wantErr: true,
		},
		{
			name:    "empty profile defaults to default",
			data:    "[default]\naws_access_key_id=KEY\naws_secret_access_key=SECRET\n",
			profile: "",
			wantAccessKey: "KEY",
			wantSecretKey: "SECRET",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds, err := ParseAWSCredentials([]byte(tt.data), tt.profile)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseAWSCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if creds.AccessKeyID != tt.wantAccessKey {
				t.Errorf("AccessKeyID = %q, want %q", creds.AccessKeyID, tt.wantAccessKey)
			}
			if creds.SecretAccessKey != tt.wantSecretKey {
				t.Errorf("SecretAccessKey = %q, want %q", creds.SecretAccessKey, tt.wantSecretKey)
			}
			if creds.SessionToken != tt.wantToken {
				t.Errorf("SessionToken = %q, want %q", creds.SessionToken, tt.wantToken)
			}
		})
	}
}

func TestUriEncode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "when input has unreserved chars, it should not encode them",
			input: "/path/AZaz09-_.~",
			want:  "/path/AZaz09-_.~",
		},
		{
			name:  "when input has spaces, it should percent-encode them not use plus",
			input: "/path/snap shot.db",
			want:  "/path/snap%20shot.db",
		},
		{
			name:  "when input has reserved chars $ & + : @, it should percent-encode them",
			input: "/key$with&special+chars:here@now",
			want:  "/key%24with%26special%2Bchars%3Ahere%40now",
		},
		{
			name:  "when input has hash and question mark, it should percent-encode them",
			input: "/path/file#1?v=2",
			want:  "/path/file%231%3Fv%3D2",
		},
		{
			name:  "when input has forward slashes, it should preserve them",
			input: "/a/b/c/d",
			want:  "/a/b/c/d",
		},
		{
			name:  "when input has equals and semicolon, it should percent-encode them",
			input: "/key=val;other",
			want:  "/key%3Dval%3Bother",
		},
		{
			name:  "when input has a percent sign, it should encode it as %25",
			input: "/100%done",
			want:  "/100%25done",
		},
		{
			name:  "when input has unicode characters, it should percent-encode each byte",
			input: "/datos/café.db",
			want:  "/datos/caf%C3%A9.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uriEncode(tt.input)
			if got != tt.want {
				t.Errorf("uriEncode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGeneratePresignedGetURL(t *testing.T) {
	// Fix time for deterministic tests
	fixedTime := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	origNow := nowFunc
	nowFunc = func() time.Time { return fixedTime }
	defer func() { nowFunc = origNow }()

	baseOpts := PresignOptions{
		Bucket:         "my-bucket",
		Key:            "path/to/snapshot.db",
		Region:         "us-east-1",
		AccessKeyID:    "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Expiry:         1 * time.Hour,
	}

	t.Run("basic virtual-hosted style URL", func(t *testing.T) {
		result, err := GeneratePresignedGetURL(baseOpts)
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

		if parsed.Host != "my-bucket.s3.us-east-1.amazonaws.com" {
			t.Errorf("unexpected host: %s", parsed.Host)
		}

		requiredParams := []string{
			"X-Amz-Algorithm",
			"X-Amz-Credential",
			"X-Amz-Date",
			"X-Amz-Expires",
			"X-Amz-Signature",
			"X-Amz-SignedHeaders",
		}
		for _, p := range requiredParams {
			if parsed.Query().Get(p) == "" {
				t.Errorf("missing required query param %s", p)
			}
		}

		if parsed.Query().Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" {
			t.Errorf("unexpected algorithm: %s", parsed.Query().Get("X-Amz-Algorithm"))
		}
		if parsed.Query().Get("X-Amz-Expires") != "3600" {
			t.Errorf("unexpected expires: %s", parsed.Query().Get("X-Amz-Expires"))
		}
	})

	t.Run("deterministic output", func(t *testing.T) {
		result1, _ := GeneratePresignedGetURL(baseOpts)
		result2, _ := GeneratePresignedGetURL(baseOpts)
		if result1 != result2 {
			t.Errorf("same inputs should produce same output")
		}
	})

	t.Run("session token adds security token param", func(t *testing.T) {
		opts := baseOpts
		opts.SessionToken = "FwoGZXIvYXdzEBYaDHqa0AP"

		result, err := GeneratePresignedGetURL(opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		parsed, _ := url.Parse(result)
		if parsed.Query().Get("X-Amz-Security-Token") == "" {
			t.Error("expected X-Amz-Security-Token in URL")
		}
	})

	t.Run("path-style addressing", func(t *testing.T) {
		opts := baseOpts
		opts.ForcePathStyle = true

		result, err := GeneratePresignedGetURL(opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		parsed, _ := url.Parse(result)
		if parsed.Host != "s3.us-east-1.amazonaws.com" {
			t.Errorf("path-style host should be s3.region.amazonaws.com, got %s", parsed.Host)
		}
		if !strings.HasPrefix(parsed.Path, "/my-bucket/") {
			t.Errorf("path-style path should start with /bucket/, got %s", parsed.Path)
		}
	})

	t.Run("custom endpoint", func(t *testing.T) {
		opts := baseOpts
		opts.Endpoint = "https://minio.example.com"

		result, err := GeneratePresignedGetURL(opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		parsed, _ := url.Parse(result)
		if parsed.Host != "minio.example.com" {
			t.Errorf("expected custom endpoint host, got %s", parsed.Host)
		}
	})

	t.Run("missing required fields", func(t *testing.T) {
		_, err := GeneratePresignedGetURL(PresignOptions{})
		if err == nil {
			t.Error("expected error for empty options")
		}
	})

	t.Run("no session token means no security token param", func(t *testing.T) {
		result, _ := GeneratePresignedGetURL(baseOpts)
		parsed, _ := url.Parse(result)
		if parsed.Query().Get("X-Amz-Security-Token") != "" {
			t.Error("should not have X-Amz-Security-Token without session token")
		}
	})
}
