package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// watchedDependencies maps Go module paths to their upstream repository URLs
// This should be kept in sync with the map in tests/integration/dependencies/dependencies_test.go
var watchedDependencies = map[string]string{
	"github.com/openshift/hypershift/api": "https://github.com/openshift/hypershift",
	// Add more dependencies here as needed:
	// "github.com/example/module": "https://github.com/example/repo",
}

func main() {
	// Read the target upstream branch from environment variable, defaulting to "release-4.20"
	targetBranch := os.Getenv("DEPS_UPSTREAM_BRANCH")
	if targetBranch == "" {
		targetBranch = "release-4.20"
	}

	fmt.Printf("🔄 Updating watched dependencies to latest %s branch versions...\n", targetBranch)

	// Find project root
	rootDir, err := findProjectRoot()
	if err != nil {
		log.Fatalf("Failed to find project root: %v", err)
	}

	// Parse dependencies from test file to ensure we're in sync
	testDeps, err := parseWatchedDependenciesFromTest(rootDir)
	if err != nil {
		log.Printf("Warning: Could not parse dependencies from test file, using hardcoded list: %v", err)
	} else {
		fmt.Printf("📋 Found %d dependencies in test file\n", len(testDeps))
		watchedDependencies = testDeps
	}

	// Update each dependency
	for module := range watchedDependencies {
		fmt.Printf("🔍 Checking %s...\n", module)

		// Get current version
		currentVersion, err := getCurrentDependencyVersion(module, rootDir)
		if err != nil {
			log.Printf("Warning: Could not get current version for %s: %v", module, err)
			continue
		}

		fmt.Printf("   Current version: %s\n", currentVersion)

		// Update to latest from target branch
		fmt.Printf("   Updating to @%s...\n", targetBranch)
		cmd := exec.Command("go", "get", module+"@"+targetBranch)
		cmd.Dir = rootDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("Error updating %s: %v\nOutput: %s", module, err, string(output))
			continue
		}

		fmt.Printf("   ✅ Updated successfully\n")
	}

	// Always run tidy and vendor to ensure consistency
	fmt.Println("🧹 Running go mod tidy and go mod vendor...")

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = rootDir
	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to run go mod tidy: %v", err)
	}

	cmd = exec.Command("go", "mod", "vendor")
	cmd.Dir = rootDir
	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to run go mod vendor: %v", err)
	}

	fmt.Println("✅ Dependency update complete!")
}

// parseWatchedDependenciesFromTest parses the watchedDependencies slice from the test file.
// It supports the []string format used in the test:
//
//	var watchedDependencies = []string{
//	    "github.com/openshift/hypershift/api",
//	}
func parseWatchedDependenciesFromTest(rootDir string) (map[string]string, error) {
	testFile := filepath.Join(rootDir, "tests", "integration", "dependencies", "dependencies_test.go")

	file, err := os.Open(testFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open test file: %w", err)
	}
	defer file.Close()

	dependencies := make(map[string]string)
	scanner := bufio.NewScanner(file)
	inWatchedDependencies := false

	// Regex to match slice entries like: "github.com/openshift/hypershift/api",
	sliceEntryRegex := regexp.MustCompile(`^\s*"([^"]+)",?\s*$`)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Start of watchedDependencies (supports both []string and map[string]string)
		if strings.Contains(line, "var watchedDependencies") && strings.Contains(line, "{") {
			inWatchedDependencies = true
			continue
		}

		// End of watchedDependencies
		if inWatchedDependencies && trimmed == "}" {
			break
		}

		// Parse dependency entries
		if inWatchedDependencies {
			// Skip comments
			if strings.HasPrefix(trimmed, "//") {
				continue
			}

			matches := sliceEntryRegex.FindStringSubmatch(line)
			if len(matches) == 2 {
				dependencies[matches[1]] = ""
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading test file: %w", err)
	}

	if len(dependencies) == 0 {
		return nil, fmt.Errorf("no dependencies found in test file")
	}

	return dependencies, nil
}

// getCurrentDependencyVersion reads the go.mod file and extracts the version
// of the specified module
func getCurrentDependencyVersion(module, rootDir string) (string, error) {
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