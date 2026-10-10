// Package atomicfile replaces files whole: the content goes to a
// uniquely named temporary file beside the target, which is synced and
// renamed over it, so a reader sees the old file or the new one and
// never a partial write, and concurrent writers never share a
// temporary.
package atomicfile

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Write replaces path with what write produces, with permissions perm.
// On any failure path is untouched and the temporary is removed.
func Write(path string, perm os.FileMode, write func(io.Writer) error) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("atomicfile: create a temporary beside %s: %w", path, err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	buf := bufio.NewWriter(f)
	err = write(buf)
	if err == nil {
		err = buf.Flush()
	}
	if err == nil {
		err = f.Chmod(perm)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("atomicfile: write %s: %w", path, err)
	}
	if err = os.Rename(tmp, path); err != nil {
		return fmt.Errorf("atomicfile: replace %s: %w", path, err)
	}
	return nil
}

// WriteFile replaces path with data, as os.WriteFile but atomically.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	return Write(path, perm, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}
