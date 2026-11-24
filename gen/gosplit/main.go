// gosplit: the Go half of splitting the generated Go package into its packages (gen/gk_split.py,
// docs/GO.md section 1). The generator writes each package's files with the flat names it always
// used (one namespace, the runtime's own lower-case names); this tool resolves every name against
// the table gk_split.py writes and rewrites it where it now lives: another package's name gets its
// package qualifier and an import, a renamed one its new name. It works on the syntax tree, so a
// field name, a struct key, a parameter or a local never changes.
//
//	gosplit refs FILE...            the top-level declarations of FILEs and the package-level
//	                                names each refers to (JSON, for gk_split.py)
//	gosplit apply TABLE FILE...     rewrite generated files in place (flat names to split ones)
//	gosplit migrate TABLE FILE...   rewrite code that imports gpuikit.dev/gk: gk.X to where X lives
//	                                now (examples, tests, the gallery's generated pages)
//	gosplit text TABLE FILE...      the same for prose (Markdown): every gk.X in the text
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: gosplit refs|apply|migrate|text ...")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "refs":
		err = refs(os.Args[2:])
	case "apply":
		err = withTable(os.Args[2], os.Args[3:], applyFile)
	case "migrate":
		err = withTable(os.Args[2], os.Args[3:], migrateFile)
	case "text":
		err = withTable(os.Args[2], os.Args[3:], textFile)
	default:
		err = fmt.Errorf("unknown mode %s", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gosplit:", err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------- refs

type decl struct {
	File    string   `json:"file"`
	I       int      `json:"i"`
	Kind    string   `json:"kind"`
	Names   []string `json:"names"`
	Recv    string   `json:"recv,omitempty"`
	Start   int      `json:"start"`
	End     int      `json:"end"`
	Refs    []string `json:"refs"`
	Imports []string `json:"imports,omitempty"`
}

func recvName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return recvName(x.X)
	case *ast.Ident:
		return x.Name
	case *ast.IndexExpr:
		return recvName(x.X)
	case *ast.IndexListExpr:
		return recvName(x.X)
	}
	return ""
}

// skipSet marks the identifiers that are no package-level name: selectors' names, struct field
// names, the keys of struct literals, parameter and result names.
func skipSet(n ast.Node) map[*ast.Ident]bool {
	skip := map[*ast.Ident]bool{}
	ast.Inspect(n, func(x ast.Node) bool {
		switch y := x.(type) {
		case *ast.SelectorExpr:
			skip[y.Sel] = true
		case *ast.CompositeLit:
			if _, isMap := y.Type.(*ast.MapType); !isMap {
				for _, el := range y.Elts {
					if kv, ok := el.(*ast.KeyValueExpr); ok {
						if id, ok := kv.Key.(*ast.Ident); ok {
							skip[id] = true
						}
					}
				}
			}
		case *ast.Field:
			for _, nm := range y.Names {
				skip[nm] = true
			}
		case *ast.FuncDecl:
			if y.Recv != nil {
				skip[y.Name] = true // a method's name is no package-level name
			}
		case *ast.LabeledStmt:
			skip[y.Label] = true
		case *ast.BranchStmt:
			if y.Label != nil {
				skip[y.Label] = true
			}
		}
		return true
	})
	return skip
}

// globalIdents calls f for every identifier of n that names something at package level (not a
// local, a parameter, a field or a selector's name).
func globalIdents(n ast.Node, top map[*ast.Object]bool, f func(*ast.Ident)) {
	skip := skipSet(n)
	ast.Inspect(n, func(x ast.Node) bool {
		id, ok := x.(*ast.Ident)
		if !ok || skip[id] || id.Name == "_" {
			return true
		}
		if id.Obj != nil && !top[id.Obj] {
			return true
		}
		f(id)
		return true
	})
}

func declStart(d ast.Decl) token.Pos {
	switch x := d.(type) {
	case *ast.FuncDecl:
		if x.Doc != nil {
			return x.Doc.Pos()
		}
	case *ast.GenDecl:
		if x.Doc != nil {
			return x.Doc.Pos()
		}
	}
	return d.Pos()
}

func refs(files []string) error {
	var out []decl
	for _, path := range files {
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		top := map[*ast.Object]bool{}
		for _, o := range af.Scope.Objects {
			top[o] = true
		}
		var imps []string
		for _, s := range af.Imports {
			imps = append(imps, s.Path.Value)
		}
		base := filepath.Base(path)
		for i, d := range af.Decls {
			dd := decl{File: base, I: i, Start: fset.Position(declStart(d)).Offset, End: fset.Position(d.End()).Offset}
			if i == 0 {
				dd.Imports = imps
			}
			switch x := d.(type) {
			case *ast.FuncDecl:
				dd.Kind = "func"
				dd.Names = []string{x.Name.Name}
				if x.Recv != nil {
					dd.Kind = "method"
					dd.Recv = recvName(x.Recv.List[0].Type)
				}
			case *ast.GenDecl:
				dd.Kind = x.Tok.String()
				if x.Tok == token.IMPORT {
					continue
				}
				for _, s := range x.Specs {
					switch sp := s.(type) {
					case *ast.TypeSpec:
						dd.Names = append(dd.Names, sp.Name.Name)
					case *ast.ValueSpec:
						for _, n := range sp.Names {
							dd.Names = append(dd.Names, n.Name)
						}
					}
				}
			}
			set := map[string]bool{}
			globalIdents(d, top, func(id *ast.Ident) {
				set[id.Name] = true
			})
			// a declaration does not refer to the names it declares
			if dd.Kind != "method" && dd.Kind != "func" {
				for _, n := range dd.Names {
					delete(set, n)
				}
			} else if dd.Kind == "func" {
				delete(set, dd.Names[0])
			}
			for k := range set {
				dd.Refs = append(dd.Refs, k)
			}
			sort.Strings(dd.Refs)
			out = append(out, dd)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(out)
}

// ---------------------------------------------------------------- the table

type table struct {
	Module string            `json:"module"` // gpuikit.dev
	Pkgs   map[string]string `json:"pkgs"`   // package name -> import path
	// Names: a flat name -> [package, name]: every generated name and every hand-written name
	// generated code or app code may give with its flat (old) spelling.
	Names map[string][2]string `json:"names"`
	// Local: unexported generated helpers every package that uses one holds its own copy of.
	Local []string `json:"local"`
	// Sels: lower-case methods and fields of the runtime's types that generated code calls, and
	// their exported names.
	Sels map[string]string `json:"sels"`
	// Public: the packages app code may import (migrate never names another).
	Public []string `json:"public"`
	// Rt: the names internal/rt declares itself (generated code there names them as they are).
	Rt []string `json:"rt"`

	local  map[string]bool
	public map[string]bool
	rt     map[string]bool
	byNew  map[string]map[string]bool
}

func withTable(path string, files []string, f func(*table, string) error) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	t := &table{}
	if err := json.Unmarshal(b, t); err != nil {
		return err
	}
	t.local = map[string]bool{}
	for _, n := range t.Local {
		t.local[n] = true
	}
	t.rt = map[string]bool{}
	for _, n := range t.Rt {
		t.rt[n] = true
	}
	t.public = map[string]bool{}
	for _, n := range t.Public {
		t.public[n] = true
	}
	var errs []string
	for _, file := range files {
		if err := f(t, file); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		if len(errs) > 40 {
			errs = append(errs[:40], fmt.Sprintf("... and %d more", len(errs)-40))
		}
		return fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	return nil
}

var predeclared = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`bool byte complex64 complex128 error float32 float64 int int8 int16 int32 int64 rune
		string uint uint8 uint16 uint32 uint64 uintptr any comparable true false iota nil append cap clear close
		complex copy delete imag len make max min new panic print println real recover C`) {
		predeclared[n] = true
	}
}

// ---------------------------------------------------------------- edits

type edit struct {
	at, end int
	text    string
	order   int
}

func applyEdits(src []byte, es []edit) []byte {
	sort.SliceStable(es, func(i, j int) bool {
		if es[i].at != es[j].at {
			return es[i].at < es[j].at
		}
		return es[i].order < es[j].order
	})
	var b bytes.Buffer
	pos := 0
	for _, e := range es {
		if e.at < pos {
			continue // overlaps an earlier edit (cannot happen for well-formed input)
		}
		b.Write(src[pos:e.at])
		b.WriteString(e.text)
		pos = e.end
	}
	b.Write(src[pos:])
	return b.Bytes()
}

// importBlock is the import declaration of a file: the given imports, std first.
func importBlock(imps map[string]string) string {
	if len(imps) == 0 {
		return ""
	}
	var std, mod []string
	for path, alias := range imps {
		line := strconv.Quote(path)
		if alias != "" {
			line = alias + " " + line
		}
		if strings.Contains(strings.SplitN(path, "/", 2)[0], ".") {
			mod = append(mod, line)
		} else {
			std = append(std, line)
		}
	}
	sort.Strings(std)
	sort.Strings(mod)
	var b strings.Builder
	b.WriteString("import (\n")
	for _, l := range std {
		b.WriteString("\t" + l + "\n")
	}
	if len(std) > 0 && len(mod) > 0 {
		b.WriteString("\n")
	}
	for _, l := range mod {
		b.WriteString("\t" + l + "\n")
	}
	b.WriteString(")")
	return b.String()
}

// replaceImports swaps the file's import declarations for imps (or adds them after the package
// clause).
func replaceImports(fset *token.FileSet, af *ast.File, imps map[string]string, es []edit) []edit {
	block := importBlock(imps)
	first := true
	for _, d := range af.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.IMPORT {
			continue
		}
		start, end := fset.Position(g.Pos()).Offset, fset.Position(g.End()).Offset
		if first {
			es = append(es, edit{at: start, end: end, text: block})
			first = false
		} else {
			es = append(es, edit{at: start, end: end, text: ""})
		}
	}
	if first && block != "" {
		at := fset.Position(af.Name.End()).Offset
		es = append(es, edit{at: at, end: at, text: "\n\n" + block})
	}
	return es
}

// usedImports: the file's imports whose name a selector uses (cgo's "C" always kept).
func usedImports(af *ast.File) map[string]string {
	names := map[string]bool{}
	ast.Inspect(af, func(x ast.Node) bool {
		if s, ok := x.(*ast.SelectorExpr); ok {
			if id, ok := s.X.(*ast.Ident); ok && id.Obj == nil {
				names[id.Name] = true
			}
		}
		return true
	})
	out := map[string]string{}
	for _, s := range af.Imports {
		path, _ := strconv.Unquote(s.Path.Value)
		name := path[strings.LastIndex(path, "/")+1:]
		alias := ""
		if s.Name != nil {
			alias = s.Name.Name
			name = alias
		}
		if path == "C" || alias == "_" || alias == "." || names[name] {
			out[path] = alias
		}
	}
	return out
}

// ---------------------------------------------------------------- apply (generated code)

func applyFile(t *table, path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return err
	}
	pkg := af.Name.Name
	off := func(p token.Pos) int { return fset.Position(p).Offset }
	top := map[*ast.Object]bool{}
	for _, o := range af.Scope.Objects {
		top[o] = true
	}
	var es []edit
	stdIdent := map[*ast.Ident]bool{}
	imps := usedImports(af)
	for p := range imps {
		if strings.HasPrefix(p, t.Module+"/") || p == t.Module {
			delete(imps, p) // recomputed below
		}
	}
	need := func(p string) string {
		if p == pkg {
			return ""
		}
		ip, ok := t.Pkgs[p]
		if !ok {
			return p + "."
		}
		imps[ip] = ""
		return p + "."
	}
	var bad []string
	// the standard packages generated code names (an import the file does not have is added)
	std := map[string]string{"time": "time", "fmt": "fmt", "math": "math", "strings": "strings", "strconv": "strconv",
		"unsafe": "unsafe", "sort": "sort", "reflect": "reflect", "os": "os", "errors": "errors", "sync": "sync",
		"utf8": "unicode/utf8", "bits": "math/bits", "slices": "slices", "atomic": "sync/atomic", "runtime": "runtime"}
	ast.Inspect(af, func(x ast.Node) bool {
		if s, ok := x.(*ast.SelectorExpr); ok {
			if id, ok := s.X.(*ast.Ident); ok && id.Obj == nil {
				if p, ok := std[id.Name]; ok {
					if _, isPkg := t.Pkgs[id.Name]; !isPkg {
						imps[p] = ""
						stdIdent[id] = true
					}
				}
			}
		}
		return true
	})
	// package-qualified names the generator wrote itself (rt.MkEl): keep, import the package
	isPkgIdent := func(id *ast.Ident) bool {
		_, ok := t.Pkgs[id.Name]
		return ok && id.Obj == nil
	}
	ast.Inspect(af, func(x ast.Node) bool {
		s, ok := x.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := s.X.(*ast.Ident); ok && isPkgIdent(id) {
			if id.Name == pkg {
				es = append(es, edit{at: off(id.Pos()), end: off(s.Sel.Pos()), text: ""})
			} else {
				need(id.Name)
			}
			return true
		}
		switch s.Sel.Name {
		case "n":
			es = append(es, edit{at: off(s.X.Pos()), end: off(s.X.Pos()), text: need("rt") + "N(", order: -1},
				edit{at: off(s.Sel.Pos()), end: off(s.Sel.End()), text: "Core)"})
		case "h":
			es = append(es, edit{at: off(s.X.Pos()), end: off(s.X.Pos()), text: need("rt") + "H(", order: -1},
				edit{at: off(s.Sel.Pos()), end: off(s.Sel.End()), text: "HCore)"})
		default:
			if n, ok := t.Sels[s.Sel.Name]; ok {
				es = append(es, edit{at: off(s.Sel.Pos()), end: off(s.Sel.End()), text: n})
			}
		}
		return true
	})
	// T{h: v}: a handle made from its number
	ast.Inspect(af, func(x ast.Node) bool {
		c, ok := x.(*ast.CompositeLit)
		if !ok || len(c.Elts) != 1 {
			return true
		}
		kv, ok := c.Elts[0].(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "h" {
			es = append(es, edit{at: off(kv.Key.Pos()), end: off(kv.Value.Pos()), text: "HCore: " + need("rt") + "MkH("},
				edit{at: off(kv.Value.End()), end: off(kv.Value.End()), text: ")"})
		}
		return true
	})
	resolve := func(id *ast.Ident, decl bool) {
		name := id.Name
		if predeclared[name] && id.Obj == nil {
			return
		}
		if t.local[name] || (pkg == "rt" && t.rt[name]) {
			return
		}
		e, ok := t.Names[name]
		if !ok {
			if decl || (id.Obj != nil && top[id.Obj]) {
				return // declared in this file and known to nobody else
			}
			bad = append(bad, fmt.Sprintf("%s: %s: no package declares %s", path, fset.Position(id.Pos()), name))
			return
		}
		q := ""
		if e[0] != pkg {
			if decl {
				bad = append(bad, fmt.Sprintf("%s: %s is declared here but lives in %s", path, name, e[0]))
				return
			}
			q = need(e[0])
		}
		if q+e[1] != name {
			es = append(es, edit{at: off(id.Pos()), end: off(id.End()), text: q + e[1]})
		}
	}
	for _, d := range af.Decls {
		switch x := d.(type) {
		case *ast.FuncDecl:
			if x.Recv == nil && x.Name.Name != "init" {
				resolve(x.Name, true)
			}
		case *ast.GenDecl:
			for _, s := range x.Specs {
				switch sp := s.(type) {
				case *ast.TypeSpec:
					resolve(sp.Name, true)
				case *ast.ValueSpec:
					for _, n := range sp.Names {
						if n.Name != "_" {
							resolve(n, true)
						}
					}
				}
			}
		}
		globalIdents(d, top, func(id *ast.Ident) {
			if isPkgIdent(id) || stdIdent[id] || id.Name == "C" {
				return
			}
			// the declared names themselves were handled above
			switch x := d.(type) {
			case *ast.FuncDecl:
				if id == x.Name {
					return
				}
			case *ast.GenDecl:
				for _, s := range x.Specs {
					switch sp := s.(type) {
					case *ast.TypeSpec:
						if id == sp.Name {
							return
						}
					case *ast.ValueSpec:
						for _, n := range sp.Names {
							if id == n {
								return
							}
						}
					}
				}
			}
			resolve(id, false)
		})
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return fmt.Errorf("%s", strings.Join(uniq(bad), "\n"))
	}
	es = replaceImports(fset, af, imps, es)
	return writeIfChanged(path, src, applyEdits(src, es))
}

func uniq(xs []string) []string {
	var out []string
	for i, x := range xs {
		if i == 0 || x != xs[i-1] {
			out = append(out, x)
		}
	}
	return out
}

func writeIfChanged(path string, old, b []byte) error {
	if bytes.Equal(old, b) {
		return nil
	}
	return os.WriteFile(path, b, 0o644)
}

// ---------------------------------------------------------------- migrate (app code)

func migrateFile(t *table, path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return err
	}
	gkName := ""
	for _, s := range af.Imports {
		if p, _ := strconv.Unquote(s.Path.Value); p == t.Module+"/gk" {
			gkName = "gk"
			if s.Name != nil {
				gkName = s.Name.Name
			}
		}
	}
	if gkName == "" || gkName == "_" || gkName == "." {
		return nil
	}
	off := func(p token.Pos) int { return fset.Position(p).Offset }
	// names the file uses for itself: an import added must not shadow one
	own := map[string]bool{}
	ast.Inspect(af, func(x ast.Node) bool {
		if id, ok := x.(*ast.Ident); ok && id.Obj != nil {
			own[id.Name] = true
		}
		return true
	})
	for _, d := range af.Decls {
		if f, ok := d.(*ast.FuncDecl); ok && f.Recv == nil {
			own[f.Name.Name] = true
		}
	}
	imps := usedImports(af)
	alias := map[string]string{} // package -> the name the file gives it
	for p, a := range imps {
		if strings.HasPrefix(p, t.Module+"/gk") {
			n := p[strings.LastIndex(p, "/")+1:]
			if p == t.Module+"/gk" {
				n = "gk"
			}
			if a != "" {
				alias[n] = a
			} else {
				alias[n] = n
			}
		}
	}
	use := map[string]bool{}
	var es []edit
	var bad []string
	ast.Inspect(af, func(x ast.Node) bool {
		s, ok := x.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := s.X.(*ast.Ident)
		if !ok || id.Name != gkName || id.Obj != nil {
			if ok && id.Obj == nil {
				for p, a := range alias {
					if _, split := t.Pkgs[p]; a == id.Name && p != "gk" && split {
						// a name of a split package that has moved since (to another package,
						// keeping its name): follow it
						if !t.has(p, s.Sel.Name) {
							if q := t.movedTo(s.Sel.Name); q != "" {
								use[q] = true
								a2, ok := alias[q]
								if !ok {
									a2 = q
									if q != "gk" && own[a2] {
										a2 = "gk" + a2
									}
									alias[q] = a2
								}
								if q == "gk" {
									a2 = gkName
								}
								es = append(es, edit{at: off(s.Pos()), end: off(s.End()), text: a2 + "." + s.Sel.Name})
								return true
							}
						}
						use[p] = true
					}
				}
			}
			return true
		}
		e, ok := t.Names[s.Sel.Name]
		if !ok || !t.public[e[0]] {
			if !ok {
				bad = append(bad, fmt.Sprintf("%s: %s.%s is no name of the split packages", fset.Position(s.Pos()), gkName, s.Sel.Name))
			}
			use["gk"] = true
			return true
		}
		use[e[0]] = true
		a, ok := alias[e[0]]
		if !ok {
			a = e[0]
			if e[0] != "gk" && own[a] {
				a = "gk" + a
			}
			alias[e[0]] = a
		}
		if e[0] == "gk" {
			a = gkName
		}
		es = append(es, edit{at: off(s.Pos()), end: off(s.End()), text: a + "." + e[1]})
		return true
	})
	if len(bad) > 0 {
		fmt.Fprintln(os.Stderr, strings.Join(bad, "\n"))
	}
	for p := range imps {
		if strings.HasPrefix(p, t.Module+"/gk") && p != t.Module+"/gk/s" && p != t.Module+"/gk/theme" && p != t.Module+"/gk/palette" {
			delete(imps, p)
		}
	}
	for p := range use {
		if _, ok := t.Pkgs[p]; !ok {
			continue // gk/s, gk/theme, gk/palette: kept above
		}
		a := alias[p]
		if p == "gk" {
			if gkName != "gk" {
				imps[t.Pkgs["gk"]] = gkName
			} else {
				imps[t.Pkgs["gk"]] = ""
			}
			continue
		}
		if a == p {
			a = ""
		}
		imps[t.Pkgs[p]] = a
	}
	es = replaceImports(fset, af, imps, es)
	return writeIfChanged(path, src, applyEdits(src, es))
}

func (t *table) index() {
	if t.byNew != nil {
		return
	}
	t.byNew = map[string]map[string]bool{}
	for _, v := range t.Names {
		if t.byNew[v[1]] == nil {
			t.byNew[v[1]] = map[string]bool{}
		}
		t.byNew[v[1]][v[0]] = true
	}
}

// has: package p declares name n now.
func (t *table) has(p, n string) bool { t.index(); return t.byNew[n][p] }

// movedTo: the one public package that declares n now ("" for none or several).
func (t *table) movedTo(n string) string {
	t.index()
	q := ""
	for p := range t.byNew[n] {
		if !t.public[p] {
			continue
		}
		if q != "" {
			return ""
		}
		q = p
	}
	return q
}

var gkRef = regexp.MustCompile(`\bgk\.([A-Z]\w*)`)

func textFile(t *table, path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out := gkRef.ReplaceAllFunc(src, func(m []byte) []byte {
		e, ok := t.Names[string(m[3:])]
		if !ok || !t.public[e[0]] {
			return m
		}
		return []byte(e[0] + "." + e[1])
	})
	return writeIfChanged(path, src, out)
}
