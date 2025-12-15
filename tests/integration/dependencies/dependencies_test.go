package dependencies

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// watchedDependencies contains Go module paths to validate for updates
// Add new dependencies here to include them in the validation checks
var watchedDependencies = []string{
	"github.com/openshift/hypershift/api",
	// Add more dependencies here as needed:
	// "github.com/example/module",
}

// TestWatchedDependenciesAreUpToDate validates that all watched dependencies
// are up-to-date with the latest versions available from their respective main branches.
// This helps prevent scheme-related issues and compatibility problems.
func TestWatchedDependenciesAreUpToDate(t *testing.T) {
	// Track overall test result
	allDependenciesUpToDate := true
	var failureMessages []string

	// Iterate through all watched dependencies
	for _, module := range watchedDependencies {
		t.Run(module, func(t *testing.T) {
			g := NewWithT(t)

			// Get current dependency version from go.mod
			currentVersion, err := getCurrentDependencyVersion(module)
			g.Expect(err).NotTo(HaveOccurred(), "Should be able to read current dependency version for %s", module)
			g.Expect(currentVersion).NotTo(BeEmpty(), "Current version should not be empty for %s", module)

			t.Logf("Current %s version: %s", module, currentVersion)

			// Get the latest available version from main branch using go list
			// This properly handles submodule dependencies by considering only commits that affect the specific module path
			latestVersion, err := getLatestDependencyVersion(module)
			g.Expect(err).NotTo(HaveOccurred(), "Should be able to get latest version from main for %s", module)

			t.Logf("Latest available %s version: %s", module, latestVersion)

			// Compare versions directly
			if currentVersion != latestVersion {
				allDependenciesUpToDate = false

				errorMsg := fmt.Sprintf("Dependency %s is not up-to-date with upstream main branch.\n"+
					"Current version: %s\n"+
					"Latest available: %s\n"+
					"Consider running: go get %s@main && go mod tidy && go mod vendor",
					module, currentVersion, latestVersion, module)
				failureMessages = append(failureMessages, errorMsg)
				t.Error(errorMsg)
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
// from the main branch using go list. This properly handles submodule dependencies
// by only considering commits that affect the specific module path.
func getLatestDependencyVersion(module string) (string, error) {
	cmd := exec.Command("go", "list", "-mod=mod", "-m", module+"@main")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get latest version: %w", err)
	}

	parts := strings.Fields(string(output))
	if len(parts) >= 2 {
		return parts[1], nil
	}

	return "", fmt.Errorf("unexpected output format from go list: %s", string(output))
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