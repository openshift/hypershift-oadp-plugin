package s3presign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const DefaultPresignExpiry = 1 * time.Hour

// nowFunc is overridden in tests for deterministic output.
var nowFunc = time.Now

// PresignOptions holds all parameters needed to generate a pre-signed S3 GET URL.
type PresignOptions struct {
	Bucket         string
	Key            string
	Region         string
	AccessKeyID    string
	SecretAccessKey string
	SessionToken   string        // optional, for STS
	Expiry         time.Duration // URL validity duration
	Endpoint       string        // custom S3 endpoint (e.g. MinIO, RHOCS)
	ForcePathStyle bool          // use path-style addressing
}

// AWSCredentials holds parsed AWS credential values.
type AWSCredentials struct {
	AccessKeyID     string
	SecretAccessKey  string
	SessionToken    string
}

// ParseS3URL parses "s3://bucket/path/to/key" into bucket and key components.
func ParseS3URL(rawURL string) (bucket, key string, err error) {
	if rawURL == "" {
		return "", "", fmt.Errorf("empty S3 URL")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid S3 URL %q: %w", rawURL, err)
	}

	if u.Scheme != "s3" {
		return "", "", fmt.Errorf("expected s3:// scheme, got %q", u.Scheme)
	}

	bucket = u.Host
	if bucket == "" {
		return "", "", fmt.Errorf("missing bucket in S3 URL %q", rawURL)
	}

	// u.Path includes leading slash, trim it
	key = strings.TrimPrefix(u.Path, "/")
	if key == "" {
		return "", "", fmt.Errorf("missing key in S3 URL %q", rawURL)
	}

	return bucket, key, nil
}

// ParseAWSCredentials parses an AWS shared credentials file (INI format)
// and returns credentials for the given profile.
func ParseAWSCredentials(data []byte, profile string) (*AWSCredentials, error) {
	if profile == "" {
		profile = "default"
	}

	lines := strings.Split(string(data), "\n")
	var inProfile bool
	creds := &AWSCredentials{}

	target := fmt.Sprintf("[%s]", profile)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// section header
		if strings.HasPrefix(line, "[") {
			inProfile = line == target
			continue
		}

		if !inProfile {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])

		switch k {
		case "aws_access_key_id":
			creds.AccessKeyID = v
		case "aws_secret_access_key":
			creds.SecretAccessKey = v
		case "aws_session_token":
			creds.SessionToken = v
		}
	}

	if creds.AccessKeyID == "" {
		return nil, fmt.Errorf("aws_access_key_id not found for profile %q", profile)
	}
	if creds.SecretAccessKey == "" {
		return nil, fmt.Errorf("aws_secret_access_key not found for profile %q", profile)
	}

	return creds, nil
}

// GeneratePresignedGetURL creates a pre-signed HTTPS GET URL using AWS SigV4.
func GeneratePresignedGetURL(opts PresignOptions) (string, error) {
	if opts.Bucket == "" || opts.Key == "" || opts.Region == "" {
		return "", fmt.Errorf("bucket, key, and region are required")
	}
	if opts.AccessKeyID == "" || opts.SecretAccessKey == "" {
		return "", fmt.Errorf("access key ID and secret access key are required")
	}

	now := nowFunc().UTC()
	datestamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")
	expirySeconds := int(opts.Expiry.Seconds())
	if expirySeconds <= 0 {
		expirySeconds = int(DefaultPresignExpiry.Seconds())
	}

	// Build the endpoint URL
	var host, urlPath string
	if opts.Endpoint != "" {
		ep := strings.TrimRight(opts.Endpoint, "/")
		parsed, err := url.Parse(ep)
		if err != nil {
			return "", fmt.Errorf("invalid endpoint %q: %w", opts.Endpoint, err)
		}
		host = parsed.Host
		urlPath = "/" + opts.Bucket + "/" + opts.Key
	} else if opts.ForcePathStyle {
		host = fmt.Sprintf("s3.%s.amazonaws.com", opts.Region)
		urlPath = "/" + opts.Bucket + "/" + opts.Key
	} else {
		host = fmt.Sprintf("%s.s3.%s.amazonaws.com", opts.Bucket, opts.Region)
		urlPath = "/" + opts.Key
	}

	credential := fmt.Sprintf("%s/%s/%s/s3/aws4_request", opts.AccessKeyID, datestamp, opts.Region)

	// Build query parameters
	params := url.Values{}
	params.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	params.Set("X-Amz-Credential", credential)
	params.Set("X-Amz-Date", amzDate)
	params.Set("X-Amz-Expires", fmt.Sprintf("%d", expirySeconds))
	params.Set("X-Amz-SignedHeaders", "host")
	if opts.SessionToken != "" {
		params.Set("X-Amz-Security-Token", opts.SessionToken)
	}

	// Canonical query string (sorted)
	canonicalQueryString := sortedQueryString(params)

	// Canonical request
	canonicalRequest := strings.Join([]string{
		"GET",
		uriEncode(urlPath),
		canonicalQueryString,
		"host:" + host + "\n",
		"host",
		"UNSIGNED-PAYLOAD",
	}, "\n")

	// String to sign
	scope := fmt.Sprintf("%s/%s/s3/aws4_request", datestamp, opts.Region)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	// Signing key
	signingKey := deriveSigningKey(opts.SecretAccessKey, datestamp, opts.Region, "s3")

	// Signature
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	params.Set("X-Amz-Signature", signature)

	scheme := "https"
	if opts.Endpoint != "" {
		if parsed, err := url.Parse(opts.Endpoint); err == nil && parsed.Scheme != "" {
			scheme = parsed.Scheme
		}
	}

	presignedURL := fmt.Sprintf("%s://%s%s?%s", scheme, host, urlPath, sortedQueryString(params))
	return presignedURL, nil
}

func deriveSigningKey(secret, datestamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(datestamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func sortedQueryString(params url.Values) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(params.Get(k)))
	}
	return strings.Join(parts, "&")
}

// uriEncode performs URI encoding per AWS SigV4 spec, preserving forward slashes.
// Uses url.QueryEscape (which encodes all reserved chars) and adjusts for SigV4:
//   - '+' (space) → '%20' (SigV4 requires percent-encoding, not '+')
//   - '%7E' → '~'  (SigV4 treats '~' as unreserved)
func uriEncode(path string) string {
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		encoded := url.QueryEscape(seg)
		encoded = strings.ReplaceAll(encoded, "+", "%20")
		encoded = strings.ReplaceAll(encoded, "%7E", "~")
		segments[i] = encoded
	}
	return strings.Join(segments, "/")
}
