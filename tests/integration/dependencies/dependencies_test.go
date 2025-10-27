package dependencies

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// watchedDependencies maps Go module paths to their upstream repository URLs
// Add new dependencies here to include them in the validation checks
var watchedDependencies = map[string]string{
	"github.com/openshift/hypershift/api": "https://github.com/openshift/hypershift",
	// Add more dependencies here as needed:
	// "github.com/example/module": "https://github.com/example/repo",
}

// TestWatchedDependenciesAreUpToDate validates that all watched dependencies
// are up-to-date with the latest commits from their respective main branches.
// This helps prevent scheme-related issues and compatibility problems.
func TestWatchedDependenciesAreUpToDate(t *testing.T) {
	// Track overall test result
	allDependenciesUpToDate := true
	var failureMessages []string

	// Iterate through all watched dependencies
	for module, repoURL := range watchedDependencies {
		t.Run(module, func(t *testing.T) {
			g := NewWithT(t)

			// Get current dependency version from go.mod
			currentVersion, err := getCurrentDependencyVersion(module)
			g.Expect(err).NotTo(HaveOccurred(), "Should be able to read current dependency version for %s", module)
			g.Expect(currentVersion).NotTo(BeEmpty(), "Current version should not be empty for %s", module)

			t.Logf("Current %s version: %s", module, currentVersion)

			// Extract commit hash from the pseudo-version
			currentCommitHash, err := extractCommitHashFromPseudoVersion(currentVersion)
			g.Expect(err).NotTo(HaveOccurred(), "Should be able to extract commit hash from pseudo-version for %s", module)

			t.Logf("Current commit hash: %s", currentCommitHash)

			// Get the latest commit hash from upstream main branch
			latestCommitHash, err := getLatestUpstreamCommitHash(repoURL)
			g.Expect(err).NotTo(HaveOccurred(), "Should be able to get latest commit from upstream for %s", module)

			t.Logf("Latest upstream commit hash: %s", latestCommitHash)

			// Compare commit hashes
			if currentCommitHash != latestCommitHash {
				allDependenciesUpToDate = false

				// Try to get the latest version for helpful information
				latestVersion, latestErr := getLatestDependencyVersion(module)
				if latestErr == nil {
					errorMsg := fmt.Sprintf("Dependency %s is not up-to-date with upstream main branch.\n"+
						"Current version: %s (commit: %s)\n"+
						"Latest available: %s (commit: %s)\n"+
						"Consider running: go get %s@main && go mod tidy && go mod vendor",
						module, currentVersion, currentCommitHash, latestVersion, latestCommitHash, module)
					failureMessages = append(failureMessages, errorMsg)
					t.Error(errorMsg)
				} else {
					errorMsg := fmt.Sprintf("Dependency %s is not up-to-date with upstream main branch.\n"+
						"Current version: %s (commit: %s)\n"+
						"Latest commit: %s\n"+
						"Consider running: go get %s@main && go mod tidy && go mod vendor",
						module, currentVersion, currentCommitHash, latestCommitHash, module)
					failureMessages = append(failureMessages, errorMsg)
					t.Error(errorMsg)
				}
			} else {
				t.Logf("✅ Dependency %s is up-to-date with upstream main branch", module)
			}
		})
	}

	// If any dependency failed, fail the main test with a summary
	if !allDependenciesUpToDate {
		t.Errorf("One or more dependencies are not up-to-date:\n%s", strings.Join(failureMessages, "\n\n"))
	} else {
		t.Logf("✅ All %d watched dependencies are up-to-date", len(watchedDependencies))
	}
}

// getCurrentDependencyVersion reads the go.mod file and extracts the version
// of the specified module
func getCurrentDependencyVersion(module string) (string, error) {
	// Get the project root directory
	rootDir, err := findProjectRoot()
	if err != nil {
		return "", fmt.Errorf("failed to find project root: %w", err)
	}

	goModPath := filepath.Join(rootDir, "go.mod")
	file, err := os.Open(goModPath)
	if err != nil {
		return "", fmt.Errorf("failed to open go.mod: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, module+" ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1], nil
			}
		}
	}

	return "", fmt.Errorf("module %s not found in go.mod", module)
}

// getLatestDependencyVersion attempts to get the latest version of the module
// from the main branch using go list
func getLatestDependencyVersion(module string) (string, error) {
	cmd := exec.Command("go", "list", "-mod=mod", "-m", "-versions", module+"@main")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get latest version: %w", err)
	}

	// The output should be just the module name for pseudo-versions
	// We need a different approach to get the actual latest pseudo-version
	cmd = exec.Command("go", "list", "-mod=readonly", "-m", module+"@main")
	output, err = cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get latest version: %w", err)
	}

	parts := strings.Fields(string(output))
	if len(parts) >= 2 {
		return parts[1], nil
	}

	return strings.TrimSpace(string(output)), nil
}

// extractCommitHashFromPseudoVersion extracts the commit hash from a Go pseudo-version
// Pseudo-versions have the format: vX.Y.Z-yyyymmddhhmmss-abcdefabcdef
func extractCommitHashFromPseudoVersion(version string) (string, error) {
	// Regex to match pseudo-version format: v0.0.0-20250108163049-830af0531d12
	re := regexp.MustCompile(`v\d+\.\d+\.\d+-\d{14}-([a-f0-9]{12})`)
	matches := re.FindStringSubmatch(version)

	if len(matches) < 2 {
		return "", fmt.Errorf("invalid pseudo-version format: %s", version)
	}

	return matches[1], nil
}

// getLatestUpstreamCommitHash gets the latest commit hash from the main branch
// of the upstream repository using git ls-remote
func getLatestUpstreamCommitHash(repoURL string) (string, error) {
	cmd := exec.Command("git", "ls-remote", repoURL, "refs/heads/main")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get latest commit from %s: %w", repoURL, err)
	}

	// Output format: "commit_hash\trefs/heads/main"
	parts := strings.Fields(string(output))
	if len(parts) < 1 {
		return "", fmt.Errorf("unexpected output format from git ls-remote: %s", string(output))
	}

	// Return first 12 characters to match pseudo-version format
	commitHash := parts[0]
	if len(commitHash) >= 12 {
		return commitHash[:12], nil
	}

	return commitHash, nil
}

// findProjectRoot finds the root directory of the Go project by looking for go.mod
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("go.mod not found")
}