package generate

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Registration wires a model into the models aggregate: a field on the Models
// struct, an entry in New, and a call in EnsureIndexes.
type Registration struct {
	Field      string // Models field, e.g. Invoice
	Package    string // model package name, e.g. invoices
	ImportPath string // e.g. example.com/app/models/invoices
	TypeName   string // model type, e.g. InvoiceModel
}

// RegisterResult reports what Register changed.
type RegisterResult struct {
	Files    map[string][]byte // absolute path → new content, only for changed files
	Done     []string          // human-readable steps performed
	Skipped  []string          // steps already present
	Warnings []string          // steps that could not be performed
}

type edit struct {
	off  int
	text string
}

type fileEdits struct {
	path    string
	src     []byte
	fset    *token.FileSet
	file    *ast.File
	edits   []edit
	imports bool // import added already
}

// Register edits the Go files in dir (the models aggregate package). It finds
// the pieces by structure rather than by marker comments, so it works on
// hand-written aggregates as well as ones gom init created.
//
// Edits are applied as text insertions at AST positions and then gofmt'ed, so
// existing comments and layout are preserved.
func Register(dir string, r Registration) (*RegisterResult, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var files []*fileEdits
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, src, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", p, err)
		}
		files = append(files, &fileEdits{path: p, src: src, fset: fset, file: f})
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s has no Go files; run `gom init` first", dir)
	}

	res := &RegisterResult{Files: map[string][]byte{}}
	alias := r.Package
	structFound, newFound, idxFound := false, false, false

	for _, fe := range files {
		// Use the import's existing name if the file already imports it.
		if name, ok := importName(fe.file, r.ImportPath); ok && name != "" {
			alias = name
		}
	}

	for _, fe := range files {
		for _, decl := range fe.file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || ts.Name.Name != "Models" {
						continue
					}
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						continue
					}
					structFound = true
					if hasField(st, r.Field) {
						res.Skipped = append(res.Skipped, "Models."+r.Field+" field")
						continue
					}
					fe.ensureImport(r.ImportPath, r.Package)
					fe.insertBefore(st.Fields.Closing, fmt.Sprintf("%s %s.%s", r.Field, alias, r.TypeName))
					res.Done = append(res.Done, "add Models."+r.Field+" field")
				}
			case *ast.FuncDecl:
				switch {
				case d.Recv == nil && d.Name.Name == "New" && d.Body != nil:
					lit := findModelsLiteral(d.Body)
					if lit == nil {
						continue
					}
					newFound = true
					if hasKey(lit, r.Field) {
						res.Skipped = append(res.Skipped, "New entry")
						continue
					}
					db := firstParam(d, "*mongo.Database")
					if db == "" {
						res.Warnings = append(res.Warnings, "New: no *mongo.Database parameter; add "+r.Field+" by hand")
						continue
					}
					fe.ensureImport(r.ImportPath, r.Package)
					value := fmt.Sprintf("*%s.New(%s)", alias, db)
					if !literalDerefs(lit) {
						value = fmt.Sprintf("%s.New(%s)", alias, db)
					}
					fe.insertBefore(lit.Rbrace, fmt.Sprintf("%s: %s,", r.Field, value))
					res.Done = append(res.Done, "add "+r.Field+" to New")
				case d.Recv != nil && d.Name.Name == "EnsureIndexes" && d.Body != nil && recvType(d) == "Models":
					idxFound = true
					recv, ctx := recvName(d), firstParam(d, "context.Context")
					call := findErrorsJoin(d.Body)
					if call == nil || recv == "" || ctx == "" {
						res.Warnings = append(res.Warnings, "EnsureIndexes: no errors.Join(...) to extend; call m."+r.Field+".EnsureIndexes(ctx) by hand")
						continue
					}
					needle := recv + "." + r.Field + ".EnsureIndexes("
					if strings.Contains(string(fe.src[fe.off(call.Lparen):fe.off(call.Rparen)]), needle) {
						res.Skipped = append(res.Skipped, "EnsureIndexes call")
						continue
					}
					fe.insertBefore(call.Rparen, fmt.Sprintf("%s.%s.EnsureIndexes(%s),", recv, r.Field, ctx))
					res.Done = append(res.Done, "call "+r.Field+".EnsureIndexes")
				}
			}
		}
	}

	if !structFound {
		res.Warnings = append(res.Warnings, "no `type Models struct` found in "+dir)
	}
	if !newFound {
		res.Warnings = append(res.Warnings, "no `func New` returning Models{...} found in "+dir)
	}
	if !idxFound {
		res.Warnings = append(res.Warnings, "no `func (m *Models) EnsureIndexes` found in "+dir)
	}

	for _, fe := range files {
		if len(fe.edits) == 0 {
			continue
		}
		out, err := fe.apply()
		if err != nil {
			return nil, err
		}
		res.Files[fe.path] = out
	}
	return res, nil
}

func (fe *fileEdits) off(p token.Pos) int { return fe.fset.Position(p).Offset }

// insertBefore puts line on its own line just before the token at p (a closing
// brace or paren), keeping whatever precedes it.
func (fe *fileEdits) insertBefore(p token.Pos, line string) {
	off := fe.off(p)
	text := "\t" + line + "\n"
	// Back up over indentation so the line goes after the previous newline.
	i := off
	for i > 0 && (fe.src[i-1] == ' ' || fe.src[i-1] == '\t') {
		i--
	}
	if i > 0 && fe.src[i-1] == '\n' {
		off = i
	} else {
		text = "\n" + text
	}
	fe.edits = append(fe.edits, edit{off: off, text: text})
}

func (fe *fileEdits) ensureImport(path, name string) {
	if fe.imports {
		return
	}
	if _, ok := importName(fe.file, path); ok {
		fe.imports = true
		return
	}
	fe.imports = true
	spec := strconv.Quote(path)
	for _, decl := range fe.file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		if gd.Lparen.IsValid() {
			fe.insertBefore(gd.Rparen, spec)
			return
		}
		// Single-line import: wrap a new one after it.
		fe.edits = append(fe.edits, edit{off: fe.off(gd.End()), text: "\nimport " + spec + "\n"})
		return
	}
	fe.edits = append(fe.edits, edit{off: fe.off(fe.file.Name.End()), text: "\n\nimport (\n\t" + spec + "\n)\n"})
}

func (fe *fileEdits) apply() ([]byte, error) {
	sort.SliceStable(fe.edits, func(i, j int) bool { return fe.edits[i].off < fe.edits[j].off })
	var b strings.Builder
	last := 0
	for _, e := range fe.edits {
		b.Write(fe.src[last:e.off])
		b.WriteString(e.text)
		last = e.off
	}
	b.Write(fe.src[last:])
	out, err := format.Source([]byte(b.String()))
	if err != nil {
		return nil, fmt.Errorf("%s: edit produced invalid Go: %w", fe.path, err)
	}
	return out, nil
}

func importName(f *ast.File, path string) (string, bool) {
	for _, imp := range f.Imports {
		if p, _ := strconv.Unquote(imp.Path.Value); p == path {
			if imp.Name != nil {
				return imp.Name.Name, true
			}
			return "", true
		}
	}
	return "", false
}

func hasField(st *ast.StructType, name string) bool {
	for _, f := range st.Fields.List {
		for _, n := range f.Names {
			if n.Name == name {
				return true
			}
		}
	}
	return false
}

func hasKey(lit *ast.CompositeLit, name string) bool {
	for _, el := range lit.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			if id, ok := kv.Key.(*ast.Ident); ok && id.Name == name {
				return true
			}
		}
	}
	return false
}

// literalDerefs reports whether existing entries are written *pkg.New(db)
// (values) rather than pkg.New(db) (pointers), defaulting to values.
func literalDerefs(lit *ast.CompositeLit) bool {
	for _, el := range lit.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			_, star := kv.Value.(*ast.StarExpr)
			return star
		}
	}
	return true
}

func findModelsLiteral(body *ast.BlockStmt) *ast.CompositeLit {
	var found *ast.CompositeLit
	ast.Inspect(body, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		if lit, ok := n.(*ast.CompositeLit); ok {
			if id, ok := lit.Type.(*ast.Ident); ok && id.Name == "Models" {
				found = lit
				return false
			}
		}
		return true
	})
	return found
}

func findErrorsJoin(body *ast.BlockStmt) *ast.CallExpr {
	var found *ast.CallExpr
	ast.Inspect(body, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Join" {
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "errors" {
					found = call
					return false
				}
			}
		}
		return true
	})
	return found
}

func exprString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprString(t.X)
	case *ast.SelectorExpr:
		return exprString(t.X) + "." + t.Sel.Name
	}
	return ""
}

func firstParam(d *ast.FuncDecl, typ string) string {
	for _, p := range d.Type.Params.List {
		if exprString(p.Type) == typ && len(p.Names) > 0 {
			return p.Names[0].Name
		}
	}
	return ""
}

func recvType(d *ast.FuncDecl) string {
	return strings.TrimPrefix(exprString(d.Recv.List[0].Type), "*")
}

func recvName(d *ast.FuncDecl) string {
	if names := d.Recv.List[0].Names; len(names) > 0 {
		return names[0].Name
	}
	return ""
}
