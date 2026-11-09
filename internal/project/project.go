// Package project locates the Go module gom is run in and loads gom.json.
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ConfigFile is read from the module root.
const ConfigFile = "gom.json"

// Config holds the directories gom generates into, relative to the module root.
type Config struct {
	Models     string `json:"models"`
	Migrations string `json:"migrations"`
	Runner     string `json:"runner"`
}

// Defaults are used for any key gom.json leaves out.
var Defaults = Config{Models: "models", Migrations: "db/migrations", Runner: "cmd/migrate"}

// Project is a Go module plus its gom configuration.
type Project struct {
	Root      string // absolute directory containing go.mod
	Module    string // module path from go.mod
	Config    Config
	HasConfig bool
}

// Find walks up from dir to the nearest go.mod and loads gom.json beside it.
func Find(dir string) (*Project, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for d := dir; ; d = filepath.Dir(d) {
		data, err := os.ReadFile(filepath.Join(d, "go.mod"))
		if err == nil {
			mod := modulePath(data)
			if mod == "" {
				return nil, fmt.Errorf("%s: no module directive", filepath.Join(d, "go.mod"))
			}
			p := &Project{Root: d, Module: mod, Config: Defaults}
			if err := p.loadConfig(); err != nil {
				return nil, err
			}
			return p, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if parent := filepath.Dir(d); parent == d {
			return nil, fmt.Errorf("no go.mod found in %s or any parent directory (run `go mod init` first)", dir)
		}
	}
}

func modulePath(gomod []byte) string {
	for _, line := range strings.Split(string(gomod), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "module"); ok && (rest == "" || rest[0] == ' ' || rest[0] == '\t') {
			rest = strings.TrimSpace(rest)
			if i := strings.Index(rest, "//"); i >= 0 {
				rest = strings.TrimSpace(rest[:i])
			}
			return strings.Trim(rest, `"`)
		}
	}
	return ""
}

func (p *Project) loadConfig() error {
	data, err := os.ReadFile(filepath.Join(p.Root, ConfigFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return fmt.Errorf("%s: %w", ConfigFile, err)
	}
	p.HasConfig = true
	if c.Models != "" {
		p.Config.Models = c.Models
	}
	if c.Migrations != "" {
		p.Config.Migrations = c.Migrations
	}
	if c.Runner != "" {
		p.Config.Runner = c.Runner
	}
	return p.Config.validate()
}

func (c Config) validate() error {
	for name, dir := range map[string]string{"models": c.Models, "migrations": c.Migrations, "runner": c.Runner} {
		clean := filepath.ToSlash(filepath.Clean(dir))
		if filepath.IsAbs(dir) || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("%s: %q must be a directory inside the module", name, dir)
		}
	}
	return nil
}

// WriteConfig saves p.Config to gom.json.
func (p *Project) WriteConfig() error {
	if err := p.Config.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p.Config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(p.Root, ConfigFile), append(data, '\n'), 0o644)
}

// Abs resolves a module-relative directory to an absolute path.
func (p *Project) Abs(rel string) string { return filepath.Join(p.Root, filepath.FromSlash(rel)) }

// ImportPath returns the import path of a module-relative directory.
func (p *Project) ImportPath(rel string) string {
	rel = path.Clean(filepath.ToSlash(rel))
	if rel == "." {
		return p.Module
	}
	return p.Module + "/" + rel
}

// Rel returns abs relative to the module root, for display.
func (p *Project) Rel(abs string) string {
	if r, err := filepath.Rel(p.Root, abs); err == nil {
		return r
	}
	return abs
}
