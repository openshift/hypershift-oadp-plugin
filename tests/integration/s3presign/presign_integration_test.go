//go:build integration

// Package s3presign_integration tests the full pre-signed URL flow against real AWS S3.
//
// The test is fully self-contained: it creates an ephemeral S3 bucket, uploads test
// objects, generates pre-signed URLs, verifies downloads, and tears everything down.
//
// Requirements:
//   - AWS credentials at ~/.aws/credentials (or path in AWS_SHARED_CREDENTIALS_FILE)
//   - aws CLI installed and in PATH
//   - Environment variables (all optional):
//     - S3_TEST_REGION: AWS region (default: us-east-1)
//     - AWS_PROFILE: credentials profile (default: default)
//     - AWS_SHARED_CREDENTIALS_FILE: path to credentials file (default: ~/.aws/credentials)
//
// Run: go test -tags integration -v -timeout 120s ./tests/integration/s3presign/
package s3presign_integration

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openshift/hypershift-oadp-plugin/pkg/s3presign"
)

const testObjectContent = "hypershift-oadp-plugin presign integration test data"

type testEnv struct {
	bucket   string
	region   string
	profile  string
	credFile string
}

// setupEnv creates an ephemeral S3 bucket and returns the test environment.
// The bucket is deleted (with all objects) when the test finishes.
func setupEnv(t *testing.T) *testEnv {
	t.Helper()

	// Check aws CLI is available
	if _, err := exec.LookPath("aws"); err != nil {
		t.Fatal("aws CLI not found in PATH — required for integration tests")
	}

	region := os.Getenv("S3_TEST_REGION")
	if region == "" {
		region = "us-east-1"
	}

	profile := os.Getenv("AWS_PROFILE")
	if profile == "" {
		profile = "default"
	}

	credFile := os.Getenv("AWS_SHARED_CREDENTIALS_FILE")
	if credFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("cannot determine home dir: %v", err)
		}
		credFile = filepath.Join(home, ".aws", "credentials")
	}

	if _, err := os.Stat(credFile); err != nil {
		t.Fatalf("credentials file %q not found: %v", credFile, err)
	}

	// Create ephemeral bucket with unique name
	bucket := fmt.Sprintf("hypershift-oadp-presign-test-%d", time.Now().UnixNano())

	env := &testEnv{
		bucket:   bucket,
		region:   region,
		profile:  profile,
		credFile: credFile,
	}

	env.createBucket(t)
	t.Cleanup(func() { env.destroyBucket(t) })

	return env
}

func (e *testEnv) awsCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("aws", args...)
	cmd.Env = append(os.Environ(),
		"AWS_SHARED_CREDENTIALS_FILE="+e.credFile,
		"AWS_DEFAULT_REGION="+e.region,
		"AWS_PROFILE="+e.profile,
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (e *testEnv) createBucket(t *testing.T) {
	t.Helper()

	args := []string{"s3", "mb", fmt.Sprintf("s3://%s", e.bucket), "--region", e.region}
	// us-east-1 doesn't accept LocationConstraint
	if e.region != "us-east-1" {
		args = append(args, "--create-bucket-configuration", fmt.Sprintf("LocationConstraint=%s", e.region))
	}

	out, err := e.awsCmd(t, args...)
	if err != nil {
		t.Fatalf("failed to create bucket %s: %v\n%s", e.bucket, err, out)
	}
	t.Logf("Created ephemeral bucket: %s", e.bucket)
}

func (e *testEnv) destroyBucket(t *testing.T) {
	t.Helper()

	// Remove all objects first
	out, err := e.awsCmd(t, "s3", "rm", fmt.Sprintf("s3://%s", e.bucket), "--recursive")
	if err != nil {
		t.Logf("WARNING: failed to empty bucket %s: %v\n%s", e.bucket, err, out)
	}

	// Delete the bucket
	out, err = e.awsCmd(t, "s3", "rb", fmt.Sprintf("s3://%s", e.bucket))
	if err != nil {
		t.Logf("WARNING: failed to delete bucket %s: %v\n%s", e.bucket, err, out)
		return
	}
	t.Logf("Destroyed ephemeral bucket: %s", e.bucket)
}

func (e *testEnv) uploadObject(t *testing.T, key, content string) {
	t.Helper()

	tmpFile := filepath.Join(t.TempDir(), "test-data")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	s3URI := fmt.Sprintf("s3://%s/%s", e.bucket, key)
	out, err := e.awsCmd(t, "s3", "cp", tmpFile, s3URI)
	if err != nil {
		t.Fatalf("failed to upload %s: %v\n%s", s3URI, err, out)
	}
	t.Logf("Uploaded: %s", s3URI)
}

func (e *testEnv) loadCredentials(t *testing.T) *s3presign.AWSCredentials {
	t.Helper()

	data, err := os.ReadFile(e.credFile)
	if err != nil {
		t.Fatalf("failed to read credentials file: %v", err)
	}

	creds, err := s3presign.ParseAWSCredentials(data, e.profile)
	if err != nil {
		t.Fatalf("failed to parse credentials: %v", err)
	}

	return creds
}

func TestPresignedURLDownload(t *testing.T) {
	env := setupEnv(t)
	creds := env.loadCredentials(t)

	objectKey := "test-snapshot.db"
	env.uploadObject(t, objectKey, testObjectContent)

	// Parse and presign
	s3URL := fmt.Sprintf("s3://%s/%s", env.bucket, objectKey)
	bucket, key, err := s3presign.ParseS3URL(s3URL)
	if err != nil {
		t.Fatalf("ParseS3URL failed: %v", err)
	}

	presignedURL, err := s3presign.GeneratePresignedGetURL(s3presign.PresignOptions{
		Bucket:         bucket,
		Key:            key,
		Region:         env.region,
		AccessKeyID:    creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:   creds.SessionToken,
		Expiry:         15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("GeneratePresignedGetURL failed: %v", err)
	}

	t.Logf("Generated pre-signed URL (first 120 chars): %.120s...", presignedURL)

	if !strings.HasPrefix(presignedURL, "https://") {
		t.Fatalf("pre-signed URL should start with https://, got: %s", presignedURL)
	}

	// Download via HTTP GET
	resp, err := http.Get(presignedURL)
	if err != nil {
		t.Fatalf("HTTP GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected HTTP 200, got %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if string(body) != testObjectContent {
		t.Errorf("content mismatch: got %q, want %q", string(body), testObjectContent)
	}

	t.Log("Successfully downloaded object via pre-signed URL, content matches")
}

func TestPresignedURLNestedKey(t *testing.T) {
	env := setupEnv(t)
	creds := env.loadCredentials(t)

	objectKey := "backups/my-backup/etcd-backup/snapshot.db"
	env.uploadObject(t, objectKey, testObjectContent)

	bucket, key, err := s3presign.ParseS3URL(fmt.Sprintf("s3://%s/%s", env.bucket, objectKey))
	if err != nil {
		t.Fatalf("ParseS3URL failed: %v", err)
	}

	presignedURL, err := s3presign.GeneratePresignedGetURL(s3presign.PresignOptions{
		Bucket:         bucket,
		Key:            key,
		Region:         env.region,
		AccessKeyID:    creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:   creds.SessionToken,
		Expiry:         15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("GeneratePresignedGetURL failed: %v", err)
	}

	resp, err := http.Get(presignedURL)
	if err != nil {
		t.Fatalf("HTTP GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected HTTP 200, got %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if string(body) != testObjectContent {
		t.Errorf("content mismatch for nested key: got %q, want %q", string(body), testObjectContent)
	}

	t.Log("Successfully downloaded nested-key object via pre-signed URL")
}

func TestPresignedURLWithSessionToken(t *testing.T) {
	env := setupEnv(t)
	creds := env.loadCredentials(t)

	if creds.SessionToken == "" {
		t.Skip("skipping: no session token in credentials (not STS)")
	}

	objectKey := "sts-test-snapshot.db"
	env.uploadObject(t, objectKey, testObjectContent)

	presignedURL, err := s3presign.GeneratePresignedGetURL(s3presign.PresignOptions{
		Bucket:         env.bucket,
		Key:            objectKey,
		Region:         env.region,
		AccessKeyID:    creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:   creds.SessionToken,
		Expiry:         15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("GeneratePresignedGetURL with session token failed: %v", err)
	}

	if !strings.Contains(presignedURL, "X-Amz-Security-Token") {
		t.Fatal("pre-signed URL should contain X-Amz-Security-Token for STS credentials")
	}

	resp, err := http.Get(presignedURL)
	if err != nil {
		t.Fatalf("HTTP GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected HTTP 200, got %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	if string(body) != testObjectContent {
		t.Errorf("content mismatch: got %q, want %q", string(body), testObjectContent)
	}

	t.Log("Successfully downloaded via pre-signed URL with STS session token")
}

func TestExpiredPresignedURLFails(t *testing.T) {
	env := setupEnv(t)
	creds := env.loadCredentials(t)

	objectKey := "expiry-test-snapshot.db"
	env.uploadObject(t, objectKey, testObjectContent)

	// Generate URL with 1-second expiry
	presignedURL, err := s3presign.GeneratePresignedGetURL(s3presign.PresignOptions{
		Bucket:         env.bucket,
		Key:            objectKey,
		Region:         env.region,
		AccessKeyID:    creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:   creds.SessionToken,
		Expiry:         1 * time.Second,
	})
	if err != nil {
		t.Fatalf("GeneratePresignedGetURL failed: %v", err)
	}

	// Wait for expiry
	t.Log("Waiting for pre-signed URL to expire...")
	time.Sleep(3 * time.Second)

	resp, err := http.Get(presignedURL)
	if err != nil {
		t.Fatalf("HTTP GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected expired URL to fail, but got HTTP 200")
	}

	t.Logf("Expired URL correctly returned HTTP %d", resp.StatusCode)
}
