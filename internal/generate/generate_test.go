package generate

import (
	"flag"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, path string, got []byte) {
	t.Helper()
	path = filepath.Join("testdata", "golden", path)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/generate -update)", err)
	}
	if string(got) != string(want) {
		t.Errorf("%s differs from golden file:\n--- got\n%s\n--- want\n%s", path, got, want)
	}
}

func TestModelGolden(t *testing.T) {
	m, err := NewModel("invoice_items", "", []string{
		"title:string!", "total:int", "company:ref", "notes:text?",
		"tags:[]string", "due_at:time?", "watchers:[]ref", "metadata:map", "amount:decimal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "InvoiceItem" || m.Package != "invoiceitems" || m.Collection != "invoiceItems" {
		t.Fatalf("names = %s %s %s", m.Name, m.Package, m.Collection)
	}
	files, err := m.Render()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		golden(t, "invoiceitems/"+f.Name+".golden", f.Content)
	}

	plain, _ := NewModel("Note", "", nil)
	files, err = plain.Render()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Name == "indexes.go" {
			golden(t, "notes/indexes.go.golden", f.Content)
		}
	}
}

func TestMigrationGolden(t *testing.T) {
	m, err := NewMigration("migrations", "BackfillInvoiceTotals", time.Date(2026, 11, 8, 20, 36, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if m.FileName() != "20261108203600_backfill_invoice_totals.go" {
		t.Fatalf("FileName = %s", m.FileName())
	}
	src, err := m.Render()
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "migration.go.golden", src)
}

// Subdocuments: a typed struct per {...}, arrays of them validated with dive,
// refs inside them indexed by dotted path.
func TestSubdocumentGolden(t *testing.T) {
	m, err := NewModel("order", "", []string{
		"number:string!",
		"address:{street:string!,city,zip:string?,geo:{lat:float,lng:float}}",
		"billing:{name:string!,vat:string?}?",
		"lines:[]{product:ref,qty:int!,price:decimal,options:[]{name,value}}!",
		"notes:[]{body:text,at:time}",
	})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range m.Structs() {
		names = append(names, s.Name)
	}
	want := "OrderAddress OrderAddressGeo OrderBilling OrderLine OrderLineOption OrderNote"
	if strings.Join(names, " ") != want {
		t.Fatalf("structs = %v, want %s", names, want)
	}
	if got := strings.Join(m.Indexes(), " "); got != "lines.productId" {
		t.Fatalf("indexes = %s, want lines.productId", got)
	}
	files, err := m.Render()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Name == "types.go" || f.Name == "indexes.go" {
			golden(t, "orders/"+f.Name+".golden", f.Content)
		}
	}
}

func TestParseFieldsErrors(t *testing.T) {
	for _, specs := range [][]string{
		{"title:strin"},
		{"id:string"},
		{"created_at:time"},
		{"title", "title:text"},
		{"9lives:int"},
		{"x:int?!"},
		{"address:{}"},
		{"address:{city:string"},
		{"address:{city:string}}"},
		{"address:{city,,zip}"},
		{"address:{city:strin}"},
		{"address:{city,city}"},
	} {
		if _, err := ParseFields("Order", specs); err == nil {
			t.Errorf("ParseFields(%q) succeeded, want error", specs)
		}
	}
	// Subdocuments may have their own id; only the model reserves it.
	if _, err := ParseFields("Order", []string{"lines:[]{id:id,qty:int}"}); err != nil {
		t.Errorf("id inside a subdocument: %v", err)
	}
}

// The shell expands an unquoted address:{city:string,zip:string} into
// address:city:string address:zip:string; say so instead of a bare type error.
func TestShellExpandedBracesHint(t *testing.T) {
	_, err := ParseFields("Order", []string{"address:city:string", "address:zip:string"})
	if err == nil || !strings.Contains(err.Error(), "quote the argument") {
		t.Fatalf("err = %v, want a hint about quoting", err)
	}
}

func TestSubdocumentNameCollision(t *testing.T) {
	if _, err := NewModel("order", "", []string{"line:{a}", "lines:[]{b}"}); err == nil {
		t.Error("line and lines both produce OrderLine; want an error")
	}
	if _, err := NewModel("order", "", []string{"model:{a}"}); err == nil {
		t.Error("model produces OrderModel, the model type itself; want an error")
	}
}

func copyFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	entries, err := os.ReadDir("testdata/register")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join("testdata/register", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func applyResult(t *testing.T, res *RegisterResult) {
	t.Helper()
	for path, b := range res.Files {
		if _, err := parser.ParseFile(token.NewFileSet(), path, b, parser.ParseComments); err != nil {
			t.Fatalf("%s does not parse: %v", path, err)
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRegister(t *testing.T) {
	dir := copyFixture(t)
	reg := Registration{Field: "Invoice", Package: "invoices", ImportPath: "example.com/app/models/invoices", TypeName: "InvoiceModel"}

	res, err := Register(dir, reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) > 0 {
		t.Fatalf("warnings: %v", res.Warnings)
	}
	if len(res.Done) != 3 || len(res.Files) != 3 {
		t.Fatalf("done = %v, files = %d; want 3 steps in 3 files", res.Done, len(res.Files))
	}
	if _, touched := res.Files[filepath.Join(dir, "methods.go")]; touched {
		t.Error("methods.go was rewritten")
	}
	applyResult(t, res)
	for _, name := range []string{"types.go", "new.go", "indexes.go"} {
		b, _ := os.ReadFile(filepath.Join(dir, name))
		golden(t, "register/"+name+".golden", b)
	}

	// Running again changes nothing.
	res, err = Register(dir, reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 0 || len(res.Skipped) != 3 {
		t.Fatalf("second run: files = %d, skipped = %v; want no changes", len(res.Files), res.Skipped)
	}
}

func TestRegisterFreshAggregate(t *testing.T) {
	dir := t.TempDir()
	files, err := Aggregate("models")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f.Name), f.Content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"Invoice", "Person"} {
		pkg := strings.ToLower(name) + "s"
		res, err := Register(dir, Registration{Field: name, Package: pkg, ImportPath: "example.com/app/models/" + pkg, TypeName: name + "Model"})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Warnings) > 0 {
			t.Fatalf("warnings: %v", res.Warnings)
		}
		applyResult(t, res)
	}
	for _, name := range []string{"types.go", "new.go", "indexes.go"} {
		b, _ := os.ReadFile(filepath.Join(dir, name))
		golden(t, "fresh/"+name+".golden", b)
	}
}

func TestRegisterWarnsWhenAggregateIsUnrecognised(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.go"), []byte("package models\n\ntype Other struct{}\n"), 0o644)
	res, err := Register(dir, Registration{Field: "Invoice", Package: "invoices", ImportPath: "x/invoices", TypeName: "InvoiceModel"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 3 || len(res.Files) != 0 {
		t.Fatalf("warnings = %v, files = %d; want 3 warnings and no edits", res.Warnings, len(res.Files))
	}
}
