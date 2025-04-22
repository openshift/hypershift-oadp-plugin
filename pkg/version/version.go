package version

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var (
	Version = getGitBranch()
	Commit  = getGitCommit()
	Date    = time.Now().Format(time.RFC3339)
)

// getGitBranch returns the current git branch
func getGitBranch() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// getGitCommit returns the current git commit hash
func getGitCommit() string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// GetVersion returns version information
func GetVersion() string {
	return fmt.Sprintf("Version: %s\nCommit: %s\nDate: %s", Version, Commit, Date)
}
