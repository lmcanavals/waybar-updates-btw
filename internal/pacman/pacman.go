// Package pacman provides functions to check for official Arch Linux package updates using checkupdates.
package pacman

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Check runs checkupdates to check for official Arch package updates.
// If fullSync is true, it runs a full check (`checkupdates --nocolor`).
// If fullSync is false, it runs a fast check (`checkupdates --nosync --change --nocolor`).
// Returns:
//   - updates: slice of update strings (e.g. "pkg 1.0 -> 2.0")
//   - ok: true if the check succeeded and the list is valid; false if no change or an error occurred
//   - err: non-nil if checkupdates returned an error exit status (other than 2)
func Check(fullSync bool) ([]string, bool, error) {
	args := []string{"checkupdates", "--nocolor"}
	if !fullSync {
		args = append(args, []string{"--nosync", "--change"}...)
	}

	cmd := exec.Command(args[0], args[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	outputStr := strings.TrimSpace(stdout.String())

	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok && exiterr.ExitCode() == 2 {
			// Exit code 2 = No updates available
			return []string{}, true, nil
		}
		errDetails := strings.TrimSpace(stderr.String())
		if errDetails != "" {
			return nil, false, fmt.Errorf("checkupdates: %w (stderr: %s)", err, errDetails)
		}
		return nil, false, fmt.Errorf("checkupdates: %w", err)
	}

	// Success (Exit Code 0)
	if outputStr == "" {
		if fullSync {
			// Full sync with empty output means 0 updates available
			return []string{}, true, nil
		}
		// Fast check (--change) with empty output means no changes since last check
		return nil, false, nil
	}

	lines := strings.Split(outputStr, "\n")
	return lines, true, nil
}
