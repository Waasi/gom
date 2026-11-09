package generate

import (
	"fmt"
	"strings"

	"github.com/waasi/gom/internal/inflect"
)

// Field is one attribute of a generated model or subdocument.
type Field struct {
	Name     string  // Go field name, e.g. CompanyID
	Key      string  // bson/json key, e.g. companyId
	Type     string  // Go type, e.g. *primitive.ObjectID, []InvoiceLine
	Optional bool    // pointer + omitempty
	Required bool    // validate:"required"
	Ref      bool    // reference to another model; indexed
	Slice    bool    // array
	Struct   *Struct // typed subdocument, or nil
	imports  []string
}

// Struct is a typed subdocument, generated as its own Go type.
type Struct struct {
	Name   string // e.g. InvoiceLine
	Fields []Field
}

// Tag renders the struct tag.
func (f Field) Tag() string {
	bson := f.Key
	if f.Optional || f.Slice || strings.HasPrefix(f.Type, "map[") {
		bson += ",omitempty"
	}
	tag := fmt.Sprintf(`json:"%s" bson:"%s"`, f.Key, bson)
	var rules []string
	if f.Required {
		rules = append(rules, "required")
	}
	if f.Slice && f.Struct != nil {
		// The validator checks fields of nested structs on its own, but only
		// looks inside slice elements when told to dive.
		if !f.Required {
			rules = append(rules, "omitempty")
		}
		rules = append(rules, "dive")
	}
	if len(rules) > 0 {
		tag += fmt.Sprintf(` validate:"%s"`, strings.Join(rules, ","))
	}
	return "`" + tag + "`"
}

type scalar struct {
	goType  string
	imports []string
}

var scalars = map[string]scalar{
	"string":   {goType: "string"},
	"text":     {goType: "string"},
	"int":      {goType: "int"},
	"integer":  {goType: "int"},
	"int32":    {goType: "int32"},
	"int64":    {goType: "int64"},
	"float":    {goType: "float64"},
	"float64":  {goType: "float64"},
	"decimal":  {goType: "primitive.Decimal128", imports: []string{pkgPrimitive}},
	"bool":     {goType: "bool"},
	"boolean":  {goType: "bool"},
	"time":     {goType: "time.Time", imports: []string{"time"}},
	"datetime": {goType: "time.Time", imports: []string{"time"}},
	"id":       {goType: "primitive.ObjectID", imports: []string{pkgPrimitive}},
	"objectid": {goType: "primitive.ObjectID", imports: []string{pkgPrimitive}},
	"map":      {goType: "map[string]any"},
	"json":     {goType: "map[string]any"},
	"any":      {goType: "any"},
}

const pkgPrimitive = "go.mongodb.org/mongo-driver/bson/primitive"

// reserved field names are generated on every model (not on subdocuments).
var reserved = map[string]bool{"ID": true, "CreatedAt": true, "UpdatedAt": true}

// ParseFields parses Rails-style attribute specs for the model named owner
// (which prefixes subdocument type names):
//
//	title                      string
//	title:string!              required
//	notes:text?                optional (pointer, omitempty)
//	company:ref                CompanyID primitive.ObjectID `bson:"companyId"`, indexed
//	tags:[]string              slice
//	address:{city:string,zip}  subdocument → type <Owner>Address
//	lines:[]{sku:string!,qty:int}  array of subdocuments → []<Owner>Line
func ParseFields(owner string, specs []string) ([]Field, error) {
	return parseFields(owner, specs, true)
}

func parseFields(owner string, specs []string, top bool) ([]Field, error) {
	var out []Field
	seen := map[string]bool{}
	for _, spec := range specs {
		f, err := parseField(owner, spec)
		if err != nil {
			return nil, err
		}
		if top && reserved[f.Name] {
			return nil, fmt.Errorf("field %q: %s is generated on every model", spec, f.Name)
		}
		if seen[f.Name] {
			return nil, fmt.Errorf("field %q: %s is declared twice", spec, f.Name)
		}
		seen[f.Name] = true
		out = append(out, f)
	}
	return out, nil
}

func parseField(owner, spec string) (Field, error) {
	name, typ, _ := strings.Cut(spec, ":")
	if typ == "" {
		typ = "string"
	}
	var f Field
	switch {
	case strings.HasSuffix(typ, "?"):
		f.Optional, typ = true, strings.TrimSuffix(typ, "?")
	case strings.HasSuffix(typ, "!"):
		f.Required, typ = true, strings.TrimSuffix(typ, "!")
	}
	if strings.HasSuffix(typ, "?") || strings.HasSuffix(typ, "!") {
		return f, fmt.Errorf("field %q: use either ? (optional) or ! (required), not both", spec)
	}
	if !isIdent(inflect.Pascal(name)) {
		return f, fmt.Errorf("field %q: invalid name", spec)
	}

	f.Slice = strings.HasPrefix(typ, "[]")
	elem := strings.TrimPrefix(typ, "[]")

	switch {
	case strings.HasPrefix(elem, "{"):
		if !strings.HasSuffix(elem, "}") {
			return f, fmt.Errorf("field %q: subdocument must end with }", spec)
		}
		// lines:[]{...} → InvoiceLine: one element of the array.
		base := name
		if f.Slice {
			base = inflect.Singularize(name)
		}
		st := &Struct{Name: owner + inflect.Pascal(base)}
		parts, err := splitTopLevel(elem[1 : len(elem)-1])
		if err != nil {
			return f, fmt.Errorf("field %q: %w", spec, err)
		}
		if len(parts) == 0 {
			return f, fmt.Errorf("field %q: subdocument has no fields", spec)
		}
		if st.Fields, err = parseFields(st.Name, parts, false); err != nil {
			return f, fmt.Errorf("in %s: %w", name, err)
		}
		f.Name, f.Key, f.Type, f.Struct = inflect.Pascal(name), inflect.Camel(name), st.Name, st

	case isRef(strings.ToLower(elem)):
		// company:ref → CompanyID; company_id:ref is accepted too.
		base := strings.TrimSuffix(inflect.Snake(name), "_id")
		if f.Slice {
			base = strings.TrimSuffix(inflect.Singularize(base), "_id")
			f.Name, f.Key = inflect.Pascal(base+"_ids"), inflect.Camel(base+"_ids")
		} else {
			f.Name, f.Key = inflect.Pascal(base+"_id"), inflect.Camel(base+"_id")
		}
		f.Type, f.Ref = "primitive.ObjectID", true
		f.imports = []string{pkgPrimitive}

	default:
		s, ok := scalars[strings.ToLower(elem)]
		if !ok {
			hint := ""
			if strings.Contains(elem, ":") {
				hint = ` — if you wrote a {...} subdocument, quote the argument so the shell does not expand the braces, e.g. "address:{city:string,zip:string}"`
			}
			return f, fmt.Errorf("field %q: unknown type %q (known: %s, ref, {...})%s", spec, elem, knownTypes(), hint)
		}
		f.Name, f.Key, f.Type = inflect.Pascal(name), inflect.Camel(name), s.goType
		f.imports = s.imports
	}

	if f.Slice {
		f.Type = "[]" + f.Type
	} else if f.Optional && f.Type != "map[string]any" && f.Type != "any" {
		// Slices, maps and any are already nilable, so ? only adds omitempty.
		f.Type = "*" + f.Type
	}
	return f, nil
}

func isRef(t string) bool { return t == "ref" || t == "references" || t == "belongs_to" }

// splitTopLevel splits s on commas that are not inside braces.
func splitTopLevel(s string) ([]string, error) {
	var parts []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced }")
			}
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced {")
	}
	if last := strings.TrimSpace(s[start:]); last != "" || len(parts) > 0 {
		parts = append(parts, last)
	}
	for _, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("empty field in subdocument")
		}
	}
	return parts, nil
}

// walkStructs visits every subdocument type, depth-first in declaration order.
func walkStructs(fields []Field, visit func(*Struct)) {
	for _, f := range fields {
		if f.Struct != nil {
			visit(f.Struct)
			walkStructs(f.Struct.Fields, visit)
		}
	}
}

// walkImports collects the imports of fields and their subdocuments.
func walkImports(fields []Field, set map[string]bool) {
	for _, f := range fields {
		for _, imp := range f.imports {
			set[imp] = true
		}
		if f.Struct != nil {
			walkImports(f.Struct.Fields, set)
		}
	}
}

// refPaths returns the dotted key of every ref field, including those inside
// subdocuments (lines.productId), for indexing.
func refPaths(fields []Field, prefix string) []string {
	var out []string
	for _, f := range fields {
		switch {
		case f.Ref:
			out = append(out, prefix+f.Key)
		case f.Struct != nil:
			out = append(out, refPaths(f.Struct.Fields, prefix+f.Key+".")...)
		}
	}
	return out
}

func knownTypes() string {
	return "string text int int32 int64 float decimal bool time id map any"
}

func isIdent(s string) bool {
	for i, r := range s {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return s != ""
}
