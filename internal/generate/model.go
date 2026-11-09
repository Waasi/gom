package generate

import (
	"bytes"
	"embed"
	"fmt"
	"go/format"
	"sort"
	"strings"
	"text/template"

	"github.com/waasi/gom/internal/inflect"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var templates = template.Must(template.New("").ParseFS(templateFS, "templates/*.tmpl"))

// GomImport is gom's module path.
const GomImport = "github.com/waasi/gom"

// ModelImport is the import path generated models use for the library.
const ModelImport = GomImport + "/model"

// Model describes a model to generate.
type Model struct {
	Name       string // Go type, e.g. InvoiceItem
	Package    string // package and directory, e.g. invoiceitems
	Collection string // MongoDB collection, e.g. invoiceItems
	Receiver   string // method receiver, e.g. i
	Fields     []Field
}

// NewModel derives every name from the user's input, which may be singular or
// plural and in any case style ("invoice_item", "InvoiceItems"), and parses
// its field specs (see ParseFields).
func NewModel(name, collection string, specs []string) (*Model, error) {
	singular := inflect.Singularize(name)
	typ := inflect.Pascal(singular)
	if !isIdent(typ) {
		return nil, fmt.Errorf("invalid model name %q", name)
	}
	fields, err := ParseFields(typ, specs)
	if err != nil {
		return nil, err
	}
	if collection == "" {
		collection = inflect.Camel(inflect.Pluralize(singular))
	}
	m := &Model{
		Name:       typ,
		Package:    inflect.Lower(inflect.Pluralize(singular)),
		Collection: collection,
		Receiver:   strings.ToLower(typ[:1]),
		Fields:     fields,
	}
	// Subdocument types share the package with the model and its Model type.
	taken := map[string]string{typ: "the model", typ + "Model": "the model's Model type"}
	var clash error
	walkStructs(fields, func(s *Struct) {
		if what, ok := taken[s.Name]; ok && clash == nil {
			clash = fmt.Errorf("subdocument type %s collides with %s; rename the field", s.Name, what)
		}
		taken[s.Name] = "another subdocument"
	})
	return m, clash
}

// Structs lists the subdocument types to declare, in declaration order.
func (m *Model) Structs() []*Struct {
	var out []*Struct
	walkStructs(m.Fields, func(s *Struct) { out = append(out, s) })
	return out
}

// Indexes are the dotted keys that get an index: every ref field, including
// those inside subdocuments.
func (m *Model) Indexes() []string { return refPaths(m.Fields, "") }

// TypesImports lists the imports types.go needs.
func (m *Model) TypesImports() []string {
	set := map[string]bool{"time": true, pkgPrimitive: true, "go.mongodb.org/mongo-driver/mongo": true}
	walkImports(m.Fields, set)
	return sortedImports(set)
}

func sortedImports(set map[string]bool) []string {
	var std, other []string
	for imp := range set {
		if strings.Contains(strings.SplitN(imp, "/", 2)[0], ".") {
			other = append(other, imp)
		} else {
			std = append(std, imp)
		}
	}
	sort.Strings(std)
	sort.Strings(other)
	if len(std) > 0 && len(other) > 0 {
		std = append(std, "")
	}
	return append(std, other...)
}

// ModelImport exposes the constant to templates.
func (m *Model) ModelImport() string { return ModelImport }

// File is a generated file, relative to the model's directory.
type File struct {
	Name    string
	Content []byte
}

var modelFiles = []string{"types.go", "new.go", "getters.go", "methods.go", "indexes.go", "queries.go"}

// Render produces the model package's files, gofmt'ed.
func (m *Model) Render() ([]File, error) {
	var out []File
	for _, name := range modelFiles {
		src, err := render(name+".tmpl", m)
		if err != nil {
			return nil, err
		}
		out = append(out, File{Name: name, Content: src})
	}
	return out, nil
}

func render(tmpl string, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, tmpl, data); err != nil {
		return nil, fmt.Errorf("%s: %w", tmpl, err)
	}
	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("%s: generated invalid Go: %w\n%s", tmpl, err, buf.Bytes())
	}
	return src, nil
}
