// Copyright 2026 Metrum AI
// SPDX-License-Identifier: Apache-2.0

package fleet

import (
	"errors"
	"os"
	"path"
	"strings"
)

func ensureSQLitePrivateMode(filePath string) error {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Chmod(filePath, 0o600)
}

func chmodSQLiteFiles(filePath string) error {
	for _, candidate := range []string{filePath, filePath + "-wal", filePath + "-shm"} {
		if _, err := os.Stat(candidate); err == nil {
			if err := os.Chmod(candidate, 0o600); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func cleanAdminReportsPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "/admin/reports"
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	prefix = path.Clean(prefix)
	if prefix == "." {
		return "/admin/reports"
	}
	return prefix
}
