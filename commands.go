package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/waasi/gom/internal/generate"
	"github.com/waasi/gom/internal/project"
)

func findProject() (*project.Project, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return project.Find(wd)
}

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	models := fs.String("models", "", "models package directory")
	migrations := fs.String("migrations", "", "migrations package directory")
	runner := fs.String("runner", "", "migration runner command directory")
	pretend := fs.Bool("pretend", false, "show what would be written")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return errUsage
	}
	p, err := findProject()
	if err != nil {
		return err
	}
	for flagVal, field := range map[*string]*string{models: &p.Config.Models, migrations: &p.Config.Migrations, runner: &p.Config.Runner} {
		if *flagVal != "" {
			*field = filepath.ToSlash(filepath.Clean(*flagVal))
		}
	}
	w := &writer{p: p, pretend: *pretend}

	w.status(map[bool]string{true: "update", false: "create"}[p.HasConfig], filepath.Join(p.Root, project.ConfigFile))
	if !*pretend {
		if err := p.WriteConfig(); err != nil {
			return err
		}
	}

	modelsDir := p.Abs(p.Config.Models)
	agg, err := generate.Aggregate(generate.PackageName(p.Config.Models))
	if err != nil {
		return err
	}
	if hasGoFiles(modelsDir) {
		w.status("exist", modelsDir)
	} else {
		for _, f := range agg {
			if err := w.createIfMissing(filepath.Join(modelsDir, f.Name), f.Content); err != nil {
				return err
			}
		}
	}

	migDir := p.Abs(p.Config.Migrations)
	doc, err := generate.MigrationsDoc(generate.PackageName(p.Config.Migrations))
	if err != nil {
		return err
	}
	if hasGoFiles(migDir) {
		w.status("exist", migDir)
	} else if err := w.createIfMissing(filepath.Join(migDir, "doc.go"), doc); err != nil {
		return err
	}

	main, err := generate.Runner(p.ImportPath(p.Config.Migrations), p.Config.Runner)
	if err != nil {
		return err
	}
	if err := w.createIfMissing(filepath.Join(p.Abs(p.Config.Runner), "main.go"), main); err != nil {
		return err
	}

	fmt.Printf("\nNext: go get %s@latest && go mod tidy\n", generate.GomImport)
	return nil
}

func hasGoFiles(dir string) bool {
	m, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	return len(m) > 0
}

func cmdModel(args []string) error {
	fs := flag.NewFlagSet("generate model", flag.ContinueOnError)
	collection := fs.String("collection", "", "MongoDB collection name (default: plural lowerCamel of the model)")
	force := fs.Bool("force", false, "overwrite existing files")
	pretend := fs.Bool("pretend", false, "show what would be written")
	skipRegister := fs.Bool("skip-register", false, "do not add the model to Models / New / EnsureIndexes")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return errUsage
	}
	fields, err := generate.ParseFields(pos[1:])
	if err != nil {
		return err
	}
	m, err := generate.NewModel(pos[0], *collection, fields)
	if err != nil {
		return err
	}
	p, err := findProject()
	if err != nil {
		return err
	}
	files, err := m.Render()
	if err != nil {
		return err
	}
	w := &writer{p: p, force: *force, pretend: *pretend}
	modelsRel := p.Config.Models
	dir := filepath.Join(p.Abs(modelsRel), m.Package)

	// Check for conflicts before writing anything, so a refusal leaves no
	// half-generated package behind.
	if !*force {
		for _, f := range files {
			path := filepath.Join(dir, f.Name)
			if old, err := os.ReadFile(path); err == nil && string(old) != string(f.Content) {
				w.status("conflict", path)
				return fmt.Errorf("%s already exists (use --force to overwrite)", p.Rel(path))
			}
		}
	}
	for _, f := range files {
		if err := w.create(filepath.Join(dir, f.Name), f.Content); err != nil {
			return err
		}
	}
	if *skipRegister {
		return nil
	}

	res, err := generate.Register(p.Abs(modelsRel), generate.Registration{
		Field:      m.Name,
		Package:    m.Package,
		ImportPath: p.ImportPath(modelsRel + "/" + m.Package),
		TypeName:   m.Name + "Model",
	})
	if err != nil {
		return fmt.Errorf("register: %w (the model files were written; use --skip-register to skip this step)", err)
	}
	paths := make([]string, 0, len(res.Files))
	for path := range res.Files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := w.update(path, res.Files[path]); err != nil {
			return err
		}
	}
	for _, s := range res.Skipped {
		fmt.Printf("%12s  %s\n", "identical", s)
	}
	for _, s := range res.Warnings {
		fmt.Fprintf(os.Stderr, "%12s  %s\n", "warning", s)
	}
	return nil
}

func cmdMigration(args []string) error {
	fs := flag.NewFlagSet("generate migration", flag.ContinueOnError)
	pretend := fs.Bool("pretend", false, "show what would be written")
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errUsage
	}
	p, err := findProject()
	if err != nil {
		return err
	}
	dir := p.Abs(p.Config.Migrations)
	m, err := generate.NewMigration(generate.PackageName(p.Config.Migrations), pos[0], time.Now())
	if err != nil {
		return err
	}
	existing, _ := filepath.Glob(filepath.Join(dir, "*_"+m.Name+".go"))
	if len(existing) > 0 {
		return fmt.Errorf("a migration named %s already exists: %s", m.Name, p.Rel(existing[0]))
	}
	src, err := m.Render()
	if err != nil {
		return err
	}
	w := &writer{p: p, pretend: *pretend}
	if !hasGoFiles(dir) {
		fmt.Fprintf(os.Stderr, "%12s  %s has no runner yet; run `gom init`\n", "note", p.Rel(dir))
	}
	return w.create(filepath.Join(dir, m.FileName()), src)
}

// cmdMigrate runs the project's runner with `go run`, since migrations are Go
// code compiled into it. A .env file in the module root fills in variables
// that are not already set.
func cmdMigrate(args []string) error {
	p, err := findProject()
	if err != nil {
		return err
	}
	runner := "./" + filepath.ToSlash(filepath.Clean(p.Config.Runner))
	if !hasGoFiles(p.Abs(p.Config.Runner)) {
		return fmt.Errorf("no migration runner in %s; run `gom init`", p.Config.Runner)
	}
	cmd := exec.Command("go", append([]string{"run", runner}, args...)...)
	cmd.Dir = p.Root
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = os.Environ()
	env, err := readDotEnv(filepath.Join(p.Root, ".env"))
	if err != nil {
		return err
	}
	for k, v := range env {
		if _, set := os.LookupEnv(k); !set {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return exitError(ee.ExitCode())
		}
		return err
	}
	return nil
}

// readDotEnv parses KEY=VALUE lines, ignoring blanks, comments and an
// optional `export ` prefix; surrounding quotes are stripped.
func readDotEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		out[k] = v
	}
	return out, sc.Err()
}
