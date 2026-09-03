package azblobsas

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var httpDo = http.DefaultClient.Do

// UserDelegationKey holds the key returned by the Azure Storage
// Get User Delegation Key API.
type UserDelegationKey struct {
	SignedOID     string `xml:"SignedOid"`
	SignedTID     string `xml:"SignedTid"`
	SignedStart   string `xml:"SignedStart"`
	SignedExpiry  string `xml:"SignedExpiry"`
	SignedService string `xml:"SignedService"`
	SignedVersion string `xml:"SignedVersion"`
	Value         string `xml:"Value"`
}

type keyInfoRequest struct {
	XMLName xml.Name `xml:"KeyInfo"`
	Start   string   `xml:"Start"`
	Expiry  string   `xml:"Expiry"`
}

// GetUserDelegationKey calls the Azure Storage REST API to obtain a user
// delegation key signed by the AAD identity represented by bearerToken.
func GetUserDelegationKey(ctx context.Context, account, bearerToken string, start, expiry time.Time, endpoint string) (*UserDelegationKey, error) {
	var host string
	if endpoint != "" {
		parsed, err := url.Parse(strings.TrimRight(endpoint, "/"))
		if err != nil {
			return nil, fmt.Errorf("invalid endpoint %q: %w", endpoint, err)
		}
		host = parsed.Scheme + "://" + parsed.Host
	} else {
		host = fmt.Sprintf("https://%s.blob.core.windows.net", account)
	}

	reqURL := host + "/?restype=service&comp=userdelegationkey"

	body, err := xml.Marshal(keyInfoRequest{
		Start:  start.UTC().Format("2006-01-02T15:04:05Z"),
		Expiry: expiry.UTC().Format("2006-01-02T15:04:05Z"),
	})
	if err != nil {
		return nil, fmt.Errorf("marshalling delegation key request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating delegation key request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	req.Header.Set("x-ms-version", sasVersion)
	req.Header.Set("Content-Type", "application/xml")

	resp, err := httpDo(req)
	if err != nil {
		return nil, fmt.Errorf("calling Get User Delegation Key: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading delegation key response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Get User Delegation Key returned %d: %s", resp.StatusCode, string(respBody))
	}

	var key UserDelegationKey
	if err := xml.Unmarshal(respBody, &key); err != nil {
		return nil, fmt.Errorf("parsing delegation key response: %w", err)
	}

	return &key, nil
}

// UserDelegationSASOptions holds parameters for generating a User Delegation SAS URL.
type UserDelegationSASOptions struct {
	Account       string
	Container     string
	Blob          string
	DelegationKey *UserDelegationKey
	Expiry        time.Duration
	Endpoint      string
}

// GenerateUserDelegationSASBlobURL creates a User Delegation SAS URL with
// read permission for a single blob.
func GenerateUserDelegationSASBlobURL(opts UserDelegationSASOptions) (string, error) {
	if opts.Account == "" || opts.Container == "" || opts.Blob == "" {
		return "", fmt.Errorf("account, container, and blob are required")
	}
	if opts.DelegationKey == nil {
		return "", fmt.Errorf("delegation key is required")
	}

	keyBytes, err := base64.StdEncoding.DecodeString(opts.DelegationKey.Value)
	if err != nil {
		return "", fmt.Errorf("invalid base64 delegation key: %w", err)
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

	// String to sign for User Delegation SAS, version 2020-12-06+
	// https://learn.microsoft.com/en-us/rest/api/storageservices/create-user-delegation-sas#version-2020-12-06-and-later
	stringToSign := strings.Join([]string{
		"r",                              // signedPermissions
		signedStart,                      // signedStart
		signedExpiry,                     // signedExpiry
		canonicalizedResource,            // canonicalizedResource
		opts.DelegationKey.SignedOID,     // signedKeyObjectId
		opts.DelegationKey.SignedTID,     // signedKeyTenantId
		opts.DelegationKey.SignedStart,   // signedKeyStart
		opts.DelegationKey.SignedExpiry,  // signedKeyExpiry
		opts.DelegationKey.SignedService, // signedKeyService
		opts.DelegationKey.SignedVersion, // signedKeyVersion
		"",                              // signedAuthorizedUserObjectId
		"",                              // signedUnauthorizedUserObjectId
		"",                              // signedCorrelationId
		"",                              // signedIP
		"https",                         // signedProtocol
		sasVersion,                      // signedVersion
		"b",                             // signedResource (blob)
		"",                              // signedSnapshotTime
		"",                              // signedEncryptionScope
		"",                              // rscc (Cache-Control)
		"",                              // rscd (Content-Disposition)
		"",                              // rsce (Content-Encoding)
		"",                              // rscl (Content-Language)
		"",                              // rsct (Content-Type)
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
	params.Set("skoid", opts.DelegationKey.SignedOID)
	params.Set("sktid", opts.DelegationKey.SignedTID)
	params.Set("skt", opts.DelegationKey.SignedStart)
	params.Set("ske", opts.DelegationKey.SignedExpiry)
	params.Set("sks", opts.DelegationKey.SignedService)
	params.Set("skv", opts.DelegationKey.SignedVersion)
	params.Set("sig", signature)

	blobPath := "/" + uriEncode(opts.Container) + "/" + uriEncode(opts.Blob)
	return fmt.Sprintf("%s://%s%s?%s", scheme, host, blobPath, params.Encode()), nil
}
