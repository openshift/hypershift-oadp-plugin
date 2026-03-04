//go:build renovate

package renovate

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// renovateConfig represents the structure of renovate.json
type renovateConfig struct {
	Schema             string        `json:"$schema"`
	Extends            []string      `json:"extends"`
	BranchPrefix       string        `json:"branchPrefix"`
	Labels             []string      `json:"labels"`
	PruneStaleBranches bool          `json:"pruneStaleBranches"`
	PackageRules       []packageRule `json:"packageRules"`
}

type packageRule struct {
	Description            string   `json:"description"`
	GroupName              string   `json:"groupName"`
	MatchPackagePrefixes   []string `json:"matchPackagePrefixes"`
	MatchManagers          []string `json:"matchManagers"`
	ExcludePackagePrefixes []string `json:"excludePackagePrefixes"`
}

func TestRenovateConfig(t *testing.T) {
	g := NewWithT(t)

	rootDir, err := findProjectRoot()
	g.Expect(err).NotTo(HaveOccurred(), "Should be able to find project root")

	data, err := os.ReadFile(filepath.Join(rootDir, "renovate.json"))
	g.Expect(err).NotTo(HaveOccurred(), "renovate.json should exist and be readable in the project root")

	var config renovateConfig
	err = json.Unmarshal(data, &config)
	g.Expect(err).NotTo(HaveOccurred(), "renovate.json should contain valid JSON")

	tests := []struct {
		name  string
		check func(t *testing.T, g Gomega, config *renovateConfig)
	}{
		{
			name: "schema reference is set",
			check: func(t *testing.T, g Gomega, config *renovateConfig) {
				g.Expect(config.Schema).To(ContainSubstring("renovatebot.com"),
					"Should have a $schema referencing renovatebot.com")
			},
		},
		{
			name: "stale branches are pruned",
			check: func(t *testing.T, g Gomega, config *renovateConfig) {
				g.Expect(config.PruneStaleBranches).To(BeTrue(),
					"pruneStaleBranches should be enabled to auto-delete merged branches")
			},
		},
		{
			name: "k8s dependencies are grouped",
			check: func(t *testing.T, g Gomega, config *renovateConfig) {
				rule := findPackageRuleByGroupName(config.PackageRules, "k8s-dependencies")
				g.Expect(rule).NotTo(BeNil(), "Should have a k8s-dependencies group")
				g.Expect(rule.MatchPackagePrefixes).To(ContainElement("k8s.io/"),
					"k8s group should match k8s.io/ packages")
				g.Expect(rule.MatchPackagePrefixes).To(ContainElement("sigs.k8s.io/"),
					"k8s group should match sigs.k8s.io/ packages")
			},
		},
		{
			name: "non-k8s Go dependencies are grouped",
			check: func(t *testing.T, g Gomega, config *renovateConfig) {
				rule := findPackageRuleByGroupName(config.PackageRules, "non-k8s-go-dependencies")
				g.Expect(rule).NotTo(BeNil(), "Should have a non-k8s-go-dependencies group")
				g.Expect(rule.MatchManagers).To(ContainElement("gomod"),
					"non-k8s group should target gomod manager")
				g.Expect(rule.ExcludePackagePrefixes).To(ContainElement("k8s.io/"),
					"non-k8s group should exclude k8s.io/ packages")
				g.Expect(rule.ExcludePackagePrefixes).To(ContainElement("sigs.k8s.io/"),
					"non-k8s group should exclude sigs.k8s.io/ packages")
			},
		},
		{
			name: "k8s prefixes excluded from non-k8s group to prevent duplicate PRs",
			check: func(t *testing.T, g Gomega, config *renovateConfig) {
				k8sRule := findPackageRuleByGroupName(config.PackageRules, "k8s-dependencies")
				nonK8sRule := findPackageRuleByGroupName(config.PackageRules, "non-k8s-go-dependencies")
				g.Expect(k8sRule).NotTo(BeNil(), "k8s-dependencies group must exist")
				g.Expect(nonK8sRule).NotTo(BeNil(), "non-k8s-go-dependencies group must exist")

				for _, prefix := range k8sRule.MatchPackagePrefixes {
					g.Expect(nonK8sRule.ExcludePackagePrefixes).To(ContainElement(prefix),
						"Prefix %s is in k8s group but not excluded from non-k8s group", prefix)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			tc.check(t, g, &config)
		})
	}
}

// TestRenovateConfigValidation runs the official renovate-config-validator via npx
// to ensure the config is accepted by Renovate without errors or migration warnings.
func TestRenovateConfigValidation(t *testing.T) {
	g := NewWithT(t)

	if _, err := exec.LookPath("npx"); err != nil {
		t.Skip("npx not found in PATH, skipping official Renovate validation")
	}

	rootDir, err := findProjectRoot()
	g.Expect(err).NotTo(HaveOccurred())

	renovatePath := filepath.Join(rootDir, "renovate.json")
	cmd := exec.Command("npx", "--yes", "--package", "renovate", "--", "renovate-config-validator", renovatePath)
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	g.Expect(err).NotTo(HaveOccurred(),
		"renovate-config-validator failed:\n%s", outputStr)
	g.Expect(outputStr).NotTo(ContainSubstring("Config migration necessary"),
		"renovate.json requires migration — update deprecated options:\n%s", outputStr)
	g.Expect(outputStr).To(ContainSubstring("Config validated successfully"),
		"Expected successful validation message in output:\n%s", outputStr)

	// Warn on any WARN lines even if validation passed
	for _, line := range strings.Split(outputStr, "\n") {
		if strings.Contains(line, "WARN") {
			t.Logf("Renovate validator warning: %s", line)
		}
	}
}

// findPackageRuleByGroupName searches for a package rule with the given group name
func findPackageRuleByGroupName(rules []packageRule, groupName string) *packageRule {
	for i := range rules {
		if rules[i].GroupName == groupName {
			return &rules[i]
		}
	}
	return nil
}

// findProjectRoot finds the root directory of the Go project by looking for go.mod
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

	return "", os.ErrNotExist
}
