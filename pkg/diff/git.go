package diff

import (
	"bytes"
	"fmt"
	"os/exec"
)

func RunGitDiff(repoRoot string, gitArgs ...string) ([]FileDiff, error) {
	args := append([]string{"diff"}, gitArgs...)
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git diff error: %w (%s)", err, stderr.String())
	}

	return ParseUnifiedDiff(&stdout)
}
