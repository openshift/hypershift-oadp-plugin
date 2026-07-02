package builders

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

var golangVersionPattern = regexp.MustCompile(`golang[_-]([\d.]+)`)

// staleOCPVersions lists OCP versions that should no longer appear in builder images.
// Update this list when adopting a newer OCP release.
var staleOCPVersions = []string{"4.21", "4.22"}

type dockerfileInfo struct {
	path           string
	golangVersions []string
}

func TestBuilderImages(t *testing.T) {
	g := NewWithT(t)

	rootDir, err := findProjectRoot()
	g.Expect(err).NotTo(HaveOccurred())

	t.Run("When checking all Dockerfiles it should use a consistent golang version", func(t *testing.T) {
		g := NewWithT(t)

		dockerfiles := []struct {
			name string
			path string
		}{
			{name: "Dockerfile", path: filepath.Join(rootDir, "Dockerfile")},
			{name: "Dockerfile.oadp", path: filepath.Join(rootDir, "Dockerfile.oadp")},
			{name: "konflux.Dockerfile", path: filepath.Join(rootDir, "konflux.Dockerfile")},
		}

		var parsedFiles []dockerfileInfo
		for _, df := range dockerfiles {
			if _, err := os.Stat(df.path); os.IsNotExist(err) {
				t.Logf("Skipping %s (not found)", df.name)
				continue
			}

			versions, err := extractGolangVersionsFromDockerfile(df.path)
			g.Expect(err).NotTo(HaveOccurred(), "Should parse %s without error", df.name)

			parsedFiles = append(parsedFiles, dockerfileInfo{
				path:           df.name,
				golangVersions: versions,
			})
			t.Logf("%s uses golang versions: %v", df.name, versions)
		}

		g.Expect(parsedFiles).NotTo(BeEmpty(), "Should find at least one Dockerfile")

		var allVersions []string
		for _, f := range parsedFiles {
			allVersions = append(allVersions, f.golangVersions...)
		}

		uniqueVersions := uniqueStrings(allVersions)
		g.Expect(uniqueVersions).To(HaveLen(1),
			"All Dockerfiles should use the same golang version, but found %v across files: %s",
			uniqueVersions, formatFileVersions(parsedFiles))
	})

	t.Run("When checking builder image references it should not use stale OCP versions", func(t *testing.T) {
		g := NewWithT(t)

		dockerfilePath := filepath.Join(rootDir, "Dockerfile")
		content, err := os.ReadFile(dockerfilePath)
		g.Expect(err).NotTo(HaveOccurred())

		for _, staleVersion := range staleOCPVersions {
			staleVersion := staleVersion
			absent := "openshift-" + staleVersion
			t.Run(fmt.Sprintf("When Dockerfile references %s it should fail because it is stale", absent), func(t *testing.T) {
				g := NewWithT(t)
				g.Expect(string(content)).NotTo(ContainSubstring(absent),
					"Dockerfile should not reference %s", absent)
			})
		}
	})
}

func TestExtractGolangVersions(t *testing.T) {
	tests := []struct {
		name              string
		dockerfileContent string
		expectedVersions  []string
	}{
		{
			name: "When Dockerfile has a single FROM with golang version it should extract it",
			dockerfileContent: `FROM registry.ci.openshift.org/openshift/release:rhel-9-release-golang-1.25-openshift-4.23 AS build
WORKDIR /app
RUN go build .`,
			expectedVersions: []string{"1.25"},
		},
		{
			name: "When Dockerfile has follow_tag and FROM with golang it should extract both",
			dockerfileContent: `#@follow_tag(registry-proxy.engineering.redhat.com/rh-osbs/openshift-golang-builder:rhel_9_golang_1.25)
FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_golang_1.25 AS builder
COPY . /workspace
FROM registry.redhat.io/ubi9/ubi-minimal:latest`,
			expectedVersions: []string{"1.25", "1.25"},
		},
		{
			name: "When Dockerfile has mismatched golang versions it should extract all of them",
			dockerfileContent: `FROM registry.ci.openshift.org/openshift/release:rhel-9-release-golang-1.25-openshift-4.23 AS build
FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_golang_1.26 AS builder2`,
			expectedVersions: []string{"1.25", "1.26"},
		},
		{
			name: "When Dockerfile has no golang builder images it should return empty",
			dockerfileContent: `FROM registry.access.redhat.com/ubi9-minimal
RUN mkdir /plugins
USER 65532:65532`,
			expectedVersions: nil,
		},
		{
			name: "When Dockerfile has only comments and no FROM lines it should return empty",
			dockerfileContent: `# This is a comment
# Another comment
WORKDIR /app`,
			expectedVersions: nil,
		},
		{
			name: "When Dockerfile has golang in a RUN line it should not extract it",
			dockerfileContent: `FROM registry.access.redhat.com/ubi9-minimal
RUN dnf install -y golang-1.25
WORKDIR /app`,
			expectedVersions: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			tmpFile, err := os.CreateTemp(t.TempDir(), "Dockerfile-*")
			g.Expect(err).NotTo(HaveOccurred())

			_, err = tmpFile.WriteString(tc.dockerfileContent)
			g.Expect(err).NotTo(HaveOccurred())
			tmpFile.Close()

			versions, err := extractGolangVersionsFromDockerfile(tmpFile.Name())
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(versions).To(Equal(tc.expectedVersions))
		})
	}
}

func TestVersionConsistencyDetection(t *testing.T) {
	tests := []struct {
		name           string
		versions       []string
		expectedUnique int
	}{
		{
			name:           "When all versions match it should report one unique version",
			versions:       []string{"1.25", "1.25", "1.25"},
			expectedUnique: 1,
		},
		{
			name:           "When versions differ it should report multiple unique versions",
			versions:       []string{"1.25", "1.26", "1.25"},
			expectedUnique: 2,
		},
		{
			name:           "When all versions are different it should report all as unique",
			versions:       []string{"1.24", "1.25", "1.26"},
			expectedUnique: 3,
		},
		{
			name:           "When there is a single version it should report one unique",
			versions:       []string{"1.25"},
			expectedUnique: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			unique := uniqueStrings(tc.versions)
			g.Expect(unique).To(HaveLen(tc.expectedUnique))
		})
	}
}

func extractGolangVersionsFromDockerfile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", path, err)
	}
	defer file.Close()

	var versions []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#@follow_tag") || strings.HasPrefix(line, "FROM") {
			matches := golangVersionPattern.FindStringSubmatch(line)
			if len(matches) >= 2 {
				versions = append(versions, matches[1])
			}
		}
	}

	return versions, scanner.Err()
}

func uniqueStrings(s []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

func formatFileVersions(files []dockerfileInfo) string {
	var parts []string
	for _, f := range files {
		parts = append(parts, fmt.Sprintf("%s=%v", f.path, f.golangVersions))
	}
	return strings.Join(parts, ", ")
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
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
