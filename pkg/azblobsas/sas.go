package azblobsas

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultSASExpiry = 1 * time.Hour
	sasVersion       = "2024-11-04"
)

var nowFunc = time.Now

// SASOptions holds all parameters needed to generate a SAS blob URL.
type SASOptions struct {
	Account    string
	Container  string
	Blob       string
	AccountKey string        // base64-encoded storage account key
	Expiry     time.Duration // URL validity duration
	Endpoint   string        // custom blob endpoint (overrides default https://<account>.blob.core.windows.net)
}

// AzureCredentials holds parsed Azure credential values.
type AzureCredentials struct {
	StorageAccountAccessKey string
}

// AADCredentials holds parsed Azure AD credential values for token-based auth.
type AADCredentials struct {
	TenantID     string
	ClientID     string
	ClientSecret string
	CloudName    string
}

// ParseAzBlobURL parses an Azure Blob Storage URL into account, container, and blob components.
// Expects format: https://<account>.blob.core.<suffix>/<container>/<blob-path>
func ParseAzBlobURL(rawURL string) (account, container, blob string, err error) {
	if rawURL == "" {
		return "", "", "", fmt.Errorf("empty Azure Blob URL")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid Azure Blob URL %q: %w", rawURL, err)
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return "", "", "", fmt.Errorf("expected https:// or http:// scheme, got %q", u.Scheme)
	}

	host := u.Hostname()
	idx := strings.Index(host, ".blob.")
	if idx <= 0 {
		return "", "", "", fmt.Errorf("hostname %q does not match <account>.blob.<suffix> pattern", host)
	}
	account = host[:idx]

	path := strings.TrimPrefix(u.Path, "/")
	if path == "" {
		return "", "", "", fmt.Errorf("missing container and blob in URL %q", rawURL)
	}

	container, blob, found := strings.Cut(path, "/")
	if !found || container == "" {
		return "", "", "", fmt.Errorf("missing blob path in URL %q", rawURL)
	}
	if blob == "" {
		return "", "", "", fmt.Errorf("empty blob path in URL %q", rawURL)
	}

	return account, container, blob, nil
}

// IsAzBlobURL returns true if rawURL is an Azure Blob Storage URL.
// It validates the scheme (http/https) and checks for the <account>.blob.<suffix> hostname pattern.
func IsAzBlobURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return false
	}
	return strings.Index(u.Hostname(), ".blob.") > 0
}

// parseDotenv parses dotenv-format data (KEY=value lines) into a map.
func parseDotenv(data []byte) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		m[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return m
}

// ParseAzureCredentials parses a dotenv-format Azure credential file
// and returns the storage account access key. keyEnvVar specifies which
// environment variable name holds the key (defaults to AZURE_STORAGE_ACCOUNT_ACCESS_KEY).
func ParseAzureCredentials(data []byte, keyEnvVar string) (*AzureCredentials, error) {
	if keyEnvVar == "" {
		keyEnvVar = "AZURE_STORAGE_ACCOUNT_ACCESS_KEY"
	}

	m := parseDotenv(data)
	key, ok := m[keyEnvVar]
	if !ok || key == "" {
		return nil, fmt.Errorf("%s not found in credential data", keyEnvVar)
	}

	return &AzureCredentials{StorageAccountAccessKey: key}, nil
}

// ParseAzureAADCredentials parses a dotenv-format Azure credential file
// and returns AAD-specific credential fields for token-based authentication.
func ParseAzureAADCredentials(data []byte) (*AADCredentials, error) {
	m := parseDotenv(data)

	tenantID := m["AZURE_TENANT_ID"]
	clientID := m["AZURE_CLIENT_ID"]
	if tenantID == "" {
		return nil, fmt.Errorf("AZURE_TENANT_ID not found in credential data")
	}
	if clientID == "" {
		return nil, fmt.Errorf("AZURE_CLIENT_ID not found in credential data")
	}

	return &AADCredentials{
		TenantID:     tenantID,
		ClientID:     clientID,
		ClientSecret: m["AZURE_CLIENT_SECRET"],
		CloudName:    m["AZURE_CLOUD_NAME"],
	}, nil
}

// GenerateSASBlobURL creates a Service SAS URL with read permission for a single blob.
func GenerateSASBlobURL(opts SASOptions) (string, error) {
	if opts.Account == "" || opts.Container == "" || opts.Blob == "" {
		return "", fmt.Errorf("account, container, and blob are required")
	}
	if opts.AccountKey == "" {
		return "", fmt.Errorf("account key is required")
	}

	keyBytes, err := base64.StdEncoding.DecodeString(opts.AccountKey)
	if err != nil {
		return "", fmt.Errorf("invalid base64 account key: %w", err)
	}

	now := nowFunc().UTC()
	expiry := opts.Expiry
	if expiry <= 0 {
		expiry = DefaultSASExpiry
	}
	expiryTime := now.Add(expiry)

	signedStart := now.Format("2006-01-02T15:04:05Z")
	signedExpiry := expiryTime.Format("2006-01-02T15:04:05Z")
	canonicalizedResource := fmt.Sprintf("/blob/%s/%s/%s", opts.Account, opts.Container, opts.Blob)

	// String to sign for service SAS, version 2020-12-06+
	// https://learn.microsoft.com/en-us/rest/api/storageservices/create-service-sas#version-2020-12-06-and-later
	stringToSign := strings.Join([]string{
		"r",                    // signedPermissions
		signedStart,            // signedStart
		signedExpiry,           // signedExpiry
		canonicalizedResource,  // canonicalizedResource
		"",                     // signedIdentifier
		"",                     // signedIP
		"https",                // signedProtocol
		sasVersion,             // signedVersion
		"b",                    // signedResource (blob)
		"",                     // signedSnapshotTime
		"",                     // signedEncryptionScope
		"",                     // rscc (Cache-Control)
		"",                     // rscd (Content-Disposition)
		"",                     // rsce (Content-Encoding)
		"",                     // rscl (Content-Language)
		"",                     // rsct (Content-Type)
	}, "\n")

	sig := hmacSHA256(keyBytes, []byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(sig)

	host, scheme := buildHostScheme(opts.Account, opts.Endpoint)

	params := url.Values{}
	params.Set("sv", sasVersion)
	params.Set("st", signedStart)
	params.Set("se", signedExpiry)
	params.Set("sr", "b")
	params.Set("sp", "r")
	params.Set("spr", "https")
	params.Set("sig", signature)

	blobPath := "/" + uriEncode(opts.Container) + "/" + uriEncode(opts.Blob)
	return fmt.Sprintf("%s://%s%s?%s", scheme, host, blobPath, params.Encode()), nil
}

func buildHostScheme(account, endpoint string) (host, scheme string) {
	scheme = "https"
	if endpoint != "" {
		if parsed, err := url.Parse(strings.TrimRight(endpoint, "/")); err == nil {
			host = parsed.Host
			if parsed.Scheme != "" {
				scheme = parsed.Scheme
			}
		}
	}
	if host == "" {
		host = fmt.Sprintf("%s.blob.core.windows.net", account)
	}
	return host, scheme
}

// uriEncode percent-encodes a path component, preserving forward slashes.
func uriEncode(s string) string {
	segments := strings.Split(s, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
