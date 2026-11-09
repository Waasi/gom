package generate

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"time"

	"github.com/waasi/gom/internal/inflect"
)

// Aggregate renders the models package that holds every model:
// types.go (Models), new.go (New) and indexes.go (EnsureIndexes).
func Aggregate(pkg string) ([]File, error) {
	var out []File
	for _, name := range []string{"types.go", "new.go", "indexes.go"} {
		src, err := render("aggregate_"+name+".tmpl", map[string]string{"Package": pkg})
		if err != nil {
			return nil, err
		}
		out = append(out, File{Name: name, Content: src})
	}
	return out, nil
}

// Runner renders the migration runner's main.go.
func Runner(migrationsImport, dir string) ([]byte, error) {
	return render("runner.go.tmpl", map[string]string{
		"Dir":        dir,
		"Migrations": migrationsImport,
		"Migrate":    GomImport + "/migrate",
	})
}

// MigrationsDoc renders the migrations package's doc.go, so the package
// compiles before the first migration exists.
func MigrationsDoc(pkg string) ([]byte, error) {
	return render("migrations_doc.go.tmpl", map[string]string{"Package": pkg})
}

// PackageName derives a Go package name from a directory: "db/migrations" →
// "migrations".
func PackageName(dir string) string {
	name := inflect.Lower(path.Base(dir))
	if name == "" || !isIdent(name) || name[0] >= '0' && name[0] <= '9' {
		return "pkg"
	}
	return name
}

// Migration is a migration file to generate.
type Migration struct {
	Package string
	Version int64
	Name    string // snake_case
	Migrate string // import path of gom/migrate
}

var migrationName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// NewMigration names a migration created at t.
func NewMigration(pkg, name string, t time.Time) (*Migration, error) {
	name = inflect.Snake(name)
	if !migrationName.MatchString(name) {
		return nil, fmt.Errorf("invalid migration name %q (use letters, digits and underscores)", name)
	}
	v, _ := strconv.ParseInt(t.UTC().Format("20060102150405"), 10, 64)
	return &Migration{Package: pkg, Version: v, Name: name, Migrate: GomImport + "/migrate"}, nil
}

// FileName is <version>_<name>.go.
func (m *Migration) FileName() string { return fmt.Sprintf("%d_%s.go", m.Version, m.Name) }

// Render produces the migration file.
func (m *Migration) Render() ([]byte, error) { return render("migration.go.tmpl", m) }
