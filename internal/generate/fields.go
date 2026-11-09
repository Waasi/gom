package generate

import (
	"fmt"
	"strings"

	"github.com/waasi/gom/internal/inflect"
)

// Field is one attribute of a generated model.
type Field struct {
	Name     string // Go field name, e.g. CompanyID
	Key      string // bson/json key, e.g. companyId
	Type     string // Go type, e.g. *primitive.ObjectID
	Optional bool   // pointer + omitempty
	Required bool   // validate:"required"
	Ref      bool   // reference to another model; indexed
	imports  []string
}

// Tag renders the struct tag.
func (f Field) Tag() string {
	bson := f.Key
	if f.Optional || strings.HasPrefix(f.Type, "[]") || strings.HasPrefix(f.Type, "map[") {
		bson += ",omitempty"
	}
	tag := fmt.Sprintf(`json:"%s" bson:"%s"`, f.Key, bson)
	if f.Required {
		tag += ` validate:"required"`
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

// reserved field names are generated on every model.
var reserved = map[string]bool{"ID": true, "CreatedAt": true, "UpdatedAt": true}

// ParseFields parses Rails-style attribute specs:
//
//	title            string
//	title:string!    required
//	notes:text?      optional (pointer, omitempty)
//	company:ref      CompanyID primitive.ObjectID `bson:"companyId"`, indexed
//	tags:[]string    slice
func ParseFields(specs []string) ([]Field, error) {
	var out []Field
	seen := map[string]bool{}
	for _, spec := range specs {
		f, err := parseField(spec)
		if err != nil {
			return nil, err
		}
		if reserved[f.Name] {
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

func parseField(spec string) (Field, error) {
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
	if inflect.Pascal(name) == "" || !isIdent(inflect.Pascal(name)) {
		return f, fmt.Errorf("field %q: invalid name", spec)
	}

	typ = strings.ToLower(typ)
	slice := strings.HasPrefix(typ, "[]")
	elem := strings.TrimPrefix(typ, "[]")

	if elem == "ref" || elem == "references" || elem == "belongs_to" {
		// company:ref → CompanyID; company_id:ref is accepted too.
		base := strings.TrimSuffix(inflect.Snake(name), "_id")
		if slice {
			base = strings.TrimSuffix(inflect.Singularize(base), "_id")
			f.Name, f.Key = inflect.Pascal(base+"_ids"), inflect.Camel(base+"_ids")
			f.Type = "[]primitive.ObjectID"
		} else {
			f.Name, f.Key = inflect.Pascal(base+"_id"), inflect.Camel(base+"_id")
			f.Type = "primitive.ObjectID"
		}
		f.Ref = true
		f.imports = []string{pkgPrimitive}
	} else {
		s, ok := scalars[elem]
		if !ok {
			return f, fmt.Errorf("field %q: unknown type %q (known: %s, ref)", spec, elem, knownTypes())
		}
		f.Name, f.Key = inflect.Pascal(name), inflect.Camel(name)
		f.Type = s.goType
		if slice {
			f.Type = "[]" + f.Type
		}
		f.imports = s.imports
	}

	// Slices, maps and any are already nilable, so ? only adds omitempty.
	if f.Optional && !slice && f.Type != "map[string]any" && f.Type != "any" {
		f.Type = "*" + f.Type
	}
	return f, nil
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
