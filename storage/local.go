/// Broadcast voice messages to a set of recipients.
/// Copyright (C) 2019 Daniel Morandini (jecoz)
///
/// This program is free software: you can redistribute it and/or modify
/// it under the terms of the GNU General Public License as published by
/// the Free Software Foundation, either version 3 of the License, or
/// (at your option) any later version.
///
/// This program is distributed in the hope that it will be useful,
/// but WITHOUT ANY WARRANTY; without even the implied warranty of
/// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
/// GNU General Public License for more details.
///
/// You should have received a copy of the GNU General Public License
/// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package storage

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Local is a local storage implementation, capable
// of writing data into local files.
type Local struct {
	// RootDir is the base directory path
	// where all the data is stored.
	RootDir string
}

// WriteRec creates a file in `RootDir`/recs/`filename` and copies
// the contents of `src` into it.
func (l *Local) WriteRec(src io.Reader, fileName string) (string, error) {
	if err := ensurePresent(l.RootDir); err != nil {
		return "", fmt.Errorf("local storage error: %v", err)
	}

	path := filepath.Join(l.RootDir, "recs", fileName)
	dest, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("local storage error: unable to create destination: %v", err)
	}

	if _, err = io.Copy(dest, src); err != nil {
		return "", fmt.Errorf("local storage error: unable to copy rec to destination: %v", err)
	}

	return path, nil
}

func (l *Local) RecFileHandler() http.Handler {
	path := filepath.Join(l.RootDir, "recs")
	return http.FileServer(http.Dir(path))
}

func ensurePresent(dir string) error {
	return os.MkdirAll(dir, os.ModePerm)
}

func (l *Local) ReadContacts(dest io.Writer) error {
	path := filepath.Join(l.RootDir, "contacts.csv")
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("local storage error: unable to open contacts file: %v", err)
	}
	defer file.Close()

	if _, err = io.Copy(dest, file); err != nil {
		return fmt.Errorf("local storage error: unable to copy contacts to destination: %v", err)
	}
	return nil
}