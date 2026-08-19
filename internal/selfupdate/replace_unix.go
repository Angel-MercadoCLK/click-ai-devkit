//go:build !windows

package selfupdate

import "fmt"

func platformPlace(tx *replacementTransaction) error {
	if err := chmodExecutable(tx.Staged, 0o755); err != nil {
		return fmt.Errorf("make staged executable: %w", err)
	}
	if err := retryFileOperation(func() error { return renameExecutable(tx.Target, tx.Backup) }); err != nil {
		return fmt.Errorf("back up executable: %w", err)
	}
	tx.State = replacementBackedUp
	if err := retryFileOperation(func() error { return renameExecutable(tx.Staged, tx.Target) }); err != nil {
		return fmt.Errorf("place executable: %w", err)
	}
	return nil
}

func platformRollback(tx *replacementTransaction) error {
	// Rename replaces the invalid target atomically on Unix, preserving no interval without an executable.
	return retryFileOperation(func() error { return renameExecutable(tx.Backup, tx.Target) })
}
