//go:build windows

package selfupdate

import (
	"fmt"
	"os"
)

func platformPlace(tx *replacementTransaction) error {
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
	if err := retryFileOperation(func() error { return removeExecutable(tx.Target) }); err != nil && !isNotExist(err) {
		return fmt.Errorf("remove invalid executable: %w", err)
	}
	return retryFileOperation(func() error { return renameExecutable(tx.Backup, tx.Target) })
}

func isNotExist(err error) bool { return err != nil && os.IsNotExist(err) }
