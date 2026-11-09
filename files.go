package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/waasi/gom/internal/project"
)

// writer writes generated files, printing a Rails-style status line for each.
type writer struct {
	p       *project.Project
	force   bool
	pretend bool
}

func (w *writer) status(verb, path string) {
	fmt.Printf("%12s  %s\n", verb, w.p.Rel(path))
}

// create writes a new file. An existing file with the same content is left
// alone; one with different content is an error unless force is set.
func (w *writer) create(path string, content []byte) error {
	old, err := os.ReadFile(path)
	switch {
	case err == nil && bytes.Equal(old, content):
		w.status("identical", path)
		return nil
	case err == nil && !w.force:
		w.status("conflict", path)
		return fmt.Errorf("%s already exists (use --force to overwrite)", w.p.Rel(path))
	case err == nil:
		w.status("force", path)
	case errors.Is(err, os.ErrNotExist):
		w.status("create", path)
	default:
		return err
	}
	return w.write(path, content)
}

// createIfMissing writes path only if it does not exist.
func (w *writer) createIfMissing(path string, content []byte) error {
	if _, err := os.Stat(path); err == nil {
		w.status("exist", path)
		return nil
	}
	w.status("create", path)
	return w.write(path, content)
}

// update overwrites a file gom edited in place.
func (w *writer) update(path string, content []byte) error {
	w.status("update", path)
	return w.write(path, content)
}

func (w *writer) write(path string, content []byte) error {
	if w.pretend {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}
