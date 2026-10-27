// Command upstreamgen extracts a self-contained copy of an upstream Go
// package and the declarations it transitively needs from the same module.
//
// It exists for upstreams that are not safe to import as a library (for
// example Traefik, whose module cannot be resolved by dependents). The
// source is downloaded at a pinned version, the entry package is copied
// whole, and from every other package of the module only the types, typed
// constants and variables, and DeepCopy methods that the entry package
// reaches are copied. Imports of the upstream module are rewritten to point
// at the generated copy.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	dirPerm  = 0o750
	filePerm = 0o600
)

type config struct {
	module    string
	version   string
	srcDir    string
	entry     string
	out       string
	outImport string
	trimPath  string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.module, "module", "", "upstream module path, e.g. github.com/traefik/traefik/v3")
	flag.StringVar(&cfg.version, "version", "", "upstream module version, e.g. v3.6.25")
	flag.StringVar(&cfg.srcDir, "src", "", "use this local checkout instead of downloading the module")
	flag.StringVar(&cfg.entry, "entry", "", "entry package, relative to the module root")
	flag.StringVar(&cfg.out, "out", "", "output directory for the generated copy")
	flag.StringVar(&cfg.outImport, "out-import", "", "import path that -out is served under")
	flag.StringVar(&cfg.trimPath, "trim", "pkg/", "prefix removed from package paths below the module root")
	flag.Parse()

	if cfg.module == "" || cfg.entry == "" || cfg.out == "" || cfg.outImport == "" {
		log.Fatal("-module, -entry, -out and -out-import are required")
	}
	if cfg.srcDir == "" {
		if cfg.version == "" {
			log.Fatal("-version is required unless -src is given")
		}
		dir, err := download(cfg.module, cfg.version)
		if err != nil {
			log.Fatalf("download: %v", err)
		}
		cfg.srcDir = dir
	}
	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

// download fetches module@version into the module cache without touching any
// go.mod, so the upstream never appears as a dependency of this repository.
func download(module, version string) (string, error) {
	tmp, err := os.MkdirTemp("", "upstreamgen")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	cmd := exec.CommandContext( //nolint:gosec // arguments are generator inputs
		context.Background(),
		"go",
		"mod",
		"download",
		"-json",
		module+"@"+version,
	)
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, stderr.String())
	}
	var info struct {
		Dir string `json:"Dir"`
	}
	if err = json.Unmarshal(raw, &info); err != nil {
		return "", err
	}
	if info.Dir == "" {
		return "", fmt.Errorf("go mod download returned no directory: %s", raw)
	}
	return info.Dir, nil
}

// ref names a type, either in the package that holds the reference
// (rel == "") or in another package of the upstream module.
type ref struct{ rel, name string }

// pkgSource is a parsed upstream package.
type pkgSource struct {
	rel     string
	fset    *token.FileSet
	files   map[string]*ast.File
	cmaps   map[string]ast.CommentMap
	types   map[string]*typeDecl
	values  []*valueDecl
	methods map[string][]*methodDecl
}

type typeDecl struct {
	file string
	decl *ast.GenDecl
	spec *ast.TypeSpec
}

type valueDecl struct {
	file string
	decl *ast.GenDecl
}

type methodDecl struct {
	file string
	decl *ast.FuncDecl
}

// generator walks the upstream module from the entry package.
type generator struct {
	cfg    config
	pkgs   map[string]*pkgSource
	needed map[string]map[string]bool
	queue  []ref
}

func run(cfg config) error {
	if err := os.RemoveAll(cfg.out); err != nil {
		return err
	}
	g := &generator{cfg: cfg, pkgs: map[string]*pkgSource{}, needed: map[string]map[string]bool{}}

	entry, err := g.load(cfg.entry)
	if err != nil {
		return err
	}
	for _, f := range entry.files {
		for _, r := range moduleRefs(cfg, f) {
			g.need(r)
		}
	}
	if err = g.resolve(); err != nil {
		return err
	}
	return g.emit(entry)
}

func (g *generator) load(rel string) (*pkgSource, error) {
	if p, ok := g.pkgs[rel]; ok {
		return p, nil
	}
	p, err := parsePackage(filepath.Join(g.cfg.srcDir, filepath.FromSlash(rel)), rel)
	if err != nil {
		return nil, err
	}
	g.pkgs[rel] = p
	return p, nil
}

func (g *generator) need(r ref) {
	if g.needed[r.rel] == nil {
		g.needed[r.rel] = map[string]bool{}
	}
	if !g.needed[r.rel][r.name] {
		g.needed[r.rel][r.name] = true
		g.queue = append(g.queue, r)
	}
}

// resolve drains the work queue until the closure of needed types is complete.
func (g *generator) resolve() error {
	for len(g.queue) > 0 {
		r := g.queue[0]
		g.queue = g.queue[1:]
		p, err := g.load(r.rel)
		if err != nil {
			return err
		}
		td, ok := p.types[r.name]
		if !ok {
			return fmt.Errorf("type %s not found in %s", r.name, r.rel)
		}
		g.follow(p, td.file, td.spec)
		for _, m := range p.methods[r.name] {
			g.follow(p, m.file, m.decl)
		}
		for _, v := range p.values {
			if valueUses(v.decl, r.name) {
				g.follow(p, v.file, v.decl)
			}
		}
	}
	return nil
}

// follow queues every type that node refers to.
func (g *generator) follow(p *pkgSource, file string, node ast.Node) {
	for _, r := range p.collect(g.cfg, file, node) {
		if r.rel == "" {
			r.rel = p.rel
		}
		g.need(r)
	}
}

func parsePackage(dir, rel string) (*pkgSource, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	p := &pkgSource{
		rel: rel, fset: token.NewFileSet(),
		files: map[string]*ast.File{}, cmaps: map[string]ast.CommentMap{},
		types: map[string]*typeDecl{}, methods: map[string][]*methodDecl{},
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(p.fset, filepath.Join(dir, n), nil, parser.ParseComments)
		if perr != nil {
			return nil, perr
		}
		p.files[n] = f
		p.cmaps[n] = ast.NewCommentMap(p.fset, f, f.Comments)
		for _, d := range f.Decls {
			p.index(n, d)
		}
	}
	return p, nil
}

// index records a top-level declaration of interest.
func (p *pkgSource) index(file string, d ast.Decl) {
	switch d := d.(type) {
	case *ast.GenDecl:
		p.indexGen(file, d)
	case *ast.FuncDecl:
		if d.Recv == nil || !strings.HasPrefix(d.Name.Name, "DeepCopy") {
			return
		}
		if base := recvBase(d); base != "" {
			p.methods[base] = append(p.methods[base], &methodDecl{file: file, decl: d})
		}
	}
}

func recvBase(d *ast.FuncDecl) string {
	if len(d.Recv.List) == 0 {
		return ""
	}
	t := d.Recv.List[0].Type
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func valueUses(d *ast.GenDecl, typeName string) bool {
	for _, s := range d.Specs {
		vs, ok := s.(*ast.ValueSpec)
		if !ok {
			continue
		}
		if id, isIdent := vs.Type.(*ast.Ident); isIdent && id.Name == typeName {
			return true
		}
	}
	return false
}

// moduleRefs lists module-internal selectors (alias.Name) used in f.
func moduleRefs(cfg config, f *ast.File) []ref {
	var out []ref
	aliases := moduleAliases(cfg, f)
	ast.Inspect(f, func(n ast.Node) bool {
		if r, ok := selectorRef(aliases, n); ok {
			out = append(out, r)
		}
		return true
	})
	return out
}

// selectorRef reports the module-internal type that n selects, if any.
func selectorRef(aliases map[string]string, n ast.Node) (ref, bool) {
	se, ok := n.(*ast.SelectorExpr)
	if !ok {
		return ref{}, false
	}
	id, ok := se.X.(*ast.Ident)
	if !ok {
		return ref{}, false
	}
	rel, ok := aliases[id.Name]
	if !ok {
		return ref{}, false
	}
	return ref{rel, se.Sel.Name}, true
}

// moduleAliases maps the local name of each module-internal import to its
// path relative to the module root.
func moduleAliases(cfg config, f *ast.File) map[string]string {
	m := map[string]string{}
	for _, imp := range f.Imports {
		ip, _ := strconv.Unquote(imp.Path.Value)
		rel, ok := strings.CutPrefix(ip, cfg.module+"/")
		if !ok {
			continue
		}
		name := path.Base(ip)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		m[name] = rel
	}
	return m
}

// collect returns the type names that node refers to, either in its own
// package (rel == "") or in another package of the module.
func (p *pkgSource) collect(cfg config, file string, node ast.Node) []ref {
	aliases := moduleAliases(cfg, p.files[file])
	var out []ref
	ast.Inspect(node, func(n ast.Node) bool {
		if r, ok := selectorRef(aliases, n); ok {
			out = append(out, r)
			return false
		}
		if id, ok := n.(*ast.Ident); ok {
			if _, known := p.types[id.Name]; known {
				out = append(out, ref{"", id.Name})
			}
		}
		return true
	})
	return out
}

// importFor translates an upstream module-relative package path to the
// import path of its generated copy.
func (c config) importFor(rel string) string {
	return path.Join(c.outImport, strings.TrimPrefix(rel, c.trimPath))
}

func (c config) dirFor(rel string) string {
	return filepath.Join(c.out, filepath.FromSlash(strings.TrimPrefix(rel, c.trimPath)))
}

func (c config) banner() string {
	v := c.version
	if v == "" {
		v = "(local)"
	}
	return fmt.Sprintf("// Code generated by upstreamgen from %s %s. DO NOT EDIT.\n\n", c.module, v)
}

// emit writes the entry package whole and a reduced file per other package.
func (g *generator) emit(entry *pkgSource) error {
	if err := g.emitEntry(entry); err != nil {
		return err
	}
	rels := make([]string, 0, len(g.needed))
	for rel := range g.needed {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		if err := g.emitPartial(g.pkgs[rel], g.needed[rel]); err != nil {
			return err
		}
	}
	return nil
}

func rewriteImports(cfg config, f *ast.File) {
	for _, imp := range f.Imports {
		ip, _ := strconv.Unquote(imp.Path.Value)
		rel, ok := strings.CutPrefix(ip, cfg.module+"/")
		if !ok {
			continue
		}
		if imp.Name == nil {
			imp.Name = ast.NewIdent(path.Base(ip))
		}
		imp.Path.Value = strconv.Quote(cfg.importFor(rel))
	}
}

func (g *generator) emitEntry(p *pkgSource) error {
	dir := g.cfg.dirFor(p.rel)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	for name, f := range p.files {
		rewriteImports(g.cfg, f)
		var buf bytes.Buffer
		buf.WriteString(g.cfg.banner())
		if err := printer.Fprint(&buf, p.fset, f); err != nil {
			return err
		}
		if err := writeFormatted(filepath.Join(dir, name), buf.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

// piece is one declaration to copy together with its source comments.
type piece struct {
	node ast.Node
	cm   ast.CommentMap
}

// partialPieces gathers the declarations to copy for the named types, and the
// imports they need keyed by "path|localname".
func partialPieces(p *pkgSource, names []string, all map[string]bool) ([]piece, map[string]*ast.ImportSpec) {
	var pieces []piece
	imports := map[string]*ast.ImportSpec{}
	add := func(file string, node ast.Node, cm ast.CommentMap) {
		pieces = append(pieces, piece{node, cm})
		addImports(imports, p.files[file], node)
	}
	for _, n := range names {
		td := p.types[n]
		spec := &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{td.spec}}
		if len(td.decl.Specs) == 1 {
			spec.Doc = td.decl.Doc
		}
		add(td.file, spec, p.cmaps[td.file])
		for _, m := range p.methods[n] {
			add(m.file, m.decl, p.cmaps[m.file])
		}
	}
	for _, v := range p.values {
		for n := range all {
			if valueUses(v.decl, n) {
				add(v.file, v.decl, p.cmaps[v.file])
				break
			}
		}
	}
	return pieces, imports
}

// addImports records the imports of file that node references.
func addImports(into map[string]*ast.ImportSpec, file *ast.File, node ast.Node) {
	used := map[string]bool{}
	ast.Inspect(node, func(n ast.Node) bool {
		if se, ok := n.(*ast.SelectorExpr); ok {
			if id, isIdent := se.X.(*ast.Ident); isIdent {
				used[id.Name] = true
			}
		}
		return true
	})
	for _, imp := range file.Imports {
		ip, _ := strconv.Unquote(imp.Path.Value)
		name := localName(ip)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if used[name] {
			cp := *imp
			lit := *imp.Path
			cp.Path = &lit
			into[ip+"|"+name] = &cp
		}
	}
}

// localName guesses the package name of an import path without a rename.
func localName(ip string) string {
	name := path.Base(ip)
	if len(name) > 1 && name[0] == 'v' && strings.Trim(name[1:], "0123456789") == "" {
		return path.Base(path.Dir(ip))
	}
	return name
}

func (g *generator) renderImports(imports map[string]*ast.ImportSpec) string {
	if len(imports) == 0 {
		return ""
	}
	keys := make([]string, 0, len(imports))
	for k := range imports {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("import (\n")
	for _, k := range keys {
		imp := imports[k]
		ip, _ := strconv.Unquote(imp.Path.Value)
		rel, internal := strings.CutPrefix(ip, g.cfg.module+"/")
		switch {
		case internal:
			local := path.Base(ip)
			if imp.Name != nil {
				local = imp.Name.Name
			}
			fmt.Fprintf(&b, "\t%s %q\n", local, g.cfg.importFor(rel))
		case imp.Name != nil:
			fmt.Fprintf(&b, "\t%s %q\n", imp.Name.Name, ip)
		default:
			fmt.Fprintf(&b, "\t%q\n", ip)
		}
	}
	b.WriteString(")\n\n")
	return b.String()
}

func (g *generator) emitPartial(p *pkgSource, set map[string]bool) error {
	dir := g.cfg.dirFor(p.rel)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	pieces, imports := partialPieces(p, names, set)

	var out bytes.Buffer
	out.WriteString(g.cfg.banner())
	fmt.Fprintf(&out, "package %s\n\n", pkgName(p))
	out.WriteString(g.renderImports(imports))
	for _, pc := range pieces {
		cn := &printer.CommentedNode{Node: pc.node, Comments: pc.cm.Filter(pc.node).Comments()}
		if err := printer.Fprint(&out, p.fset, cn); err != nil {
			return err
		}
		out.WriteString("\n\n")
	}
	return writeFormatted(filepath.Join(dir, "zz_extracted.go"), out.Bytes())
}

func pkgName(p *pkgSource) string {
	for _, f := range p.files {
		return f.Name.Name
	}
	return path.Base(p.rel)
}

func writeFormatted(file string, src []byte) error {
	formatted, err := format.Source(src)
	if err != nil {
		return errors.Join(fmt.Errorf("format %s", file), err)
	}
	return os.WriteFile(file, formatted, filePerm)
}

// indexGen records the types, constants and variables of a GenDecl.
func (p *pkgSource) indexGen(file string, d *ast.GenDecl) {
	if d.Tok == token.TYPE {
		for _, s := range d.Specs {
			if ts, ok := s.(*ast.TypeSpec); ok {
				p.types[ts.Name.Name] = &typeDecl{file: file, decl: d, spec: ts}
			}
		}
		return
	}
	if d.Tok == token.CONST || d.Tok == token.VAR {
		p.values = append(p.values, &valueDecl{file: file, decl: d})
	}
}
