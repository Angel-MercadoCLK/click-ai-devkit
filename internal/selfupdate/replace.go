package selfupdate

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var (
	renameExecutable  = os.Rename
	removeExecutable  = os.Remove
	chmodExecutable   = os.Chmod
	runVersionCommand = executeVersionCommand
)

type replacementState uint8

const (
	replacementPrepared replacementState = iota
	replacementBackedUp
	replacementPlaced
	replacementValidated
	replacementRolledBack
)

type replacementTransaction struct {
	Target, Staged, Backup, Current, Expected string
	State                                     replacementState
}

func replaceAndValidate(target, staged, current, expected string) error {
	tx := &replacementTransaction{Target: target, Staged: staged, Backup: target + ".old", Current: current, Expected: expected, State: replacementPrepared}
	lock, err := acquireTargetLock(target)
	if err != nil {
		return fmt.Errorf("lock replacement target: %w", err)
	}
	defer lock.Close()
	if err := cleanupStaleArtifacts(target); err != nil {
		return fmt.Errorf("prepare replacement: %w", err)
	}
	if err := platformPlace(tx); err != nil {
		_ = removeExecutable(staged)
		if tx.State != replacementBackedUp {
			return fmt.Errorf("back up current executable: %w", err)
		}
		return rollbackAndValidate(tx, err)
	}
	tx.State = replacementPlaced
	if err := validateExecutable(target, expected); err != nil {
		return rollbackAndValidate(tx, fmt.Errorf("validate replacement: %w", err))
	}
	tx.State = replacementValidated
	// A validated executable is committed. Cleanup failure deliberately cannot roll it back.
	_ = cleanupBackup(target)
	return nil
}

func rollbackAndValidate(tx *replacementTransaction, cause error) error {
	if err := platformRollback(tx); err != nil {
		return fmt.Errorf("replacement failed (%v); rollback failed (%v): manual recovery required; restore %s to %s", cause, err, tx.Backup, tx.Target)
	}
	tx.State = replacementRolledBack
	if err := validateExecutable(tx.Target, tx.Current); err != nil {
		return fmt.Errorf("replacement failed (%v); restored executable validation failed (%v): manual recovery required", cause, err)
	}
	return fmt.Errorf("replacement failed and the previous executable was restored: %w", cause)
}

func executeVersionCommand(path string) ([]byte, error) {
	return exec.Command(path, "--version").Output()
}

func validateExecutable(path, expected string) error {
	output, err := runVersionCommand(path)
	if err != nil {
		return fmt.Errorf("run version command: %w", err)
	}
	actual, err := parseVersionOutput(output)
	if err != nil {
		return err
	}
	if order, comparable := compareVersions(actual, expected); !comparable || order != 0 {
		return fmt.Errorf("version mismatch: got %q, expected %q", actual, expected)
	}
	return nil
}

func parseVersionOutput(output []byte) (string, error) {
	line := strings.TrimSpace(string(output))
	if line == "" || strings.ContainsAny(line, "\r\n") {
		return "", errors.New("version output must be one line")
	}
	fields := strings.Fields(line)
	if len(fields) != 3 || fields[0] != "click" || fields[1] != "version" {
		return "", fmt.Errorf("invalid version output: %q", line)
	}
	if _, comparable := compareVersions(fields[2], fields[2]); !comparable {
		return "", fmt.Errorf("invalid numeric version: %q", fields[2])
	}
	return fields[2], nil
}
