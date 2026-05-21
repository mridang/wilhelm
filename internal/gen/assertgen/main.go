// Command assertgen generates partial-match assertion structs for the
// exported struct types in a list of Go packages. It is invoked from
// `go:generate` directives in `assert/` and per-CRD subpackages.
package main

import (
	"flag"
	"fmt"
	"go/format"
	"go/types"
	"log"
	"os"
	"slices"
	"strings"

	"github.com/dave/jennifer/jen"
	"golang.org/x/tools/go/packages"
)

const (
	generatedFilePerm = 0o600
	rawDumpFilePerm   = 0o600
)

// defaultPackages is the package list scanned when --packages is not given.
// Mirrors the original zitadel-charts hardcoded set.
func defaultPackages() []string {
	return []string{
		"k8s.io/api/apps/v1",
		"k8s.io/api/autoscaling/v2",
		"k8s.io/api/batch/v1",
		"k8s.io/api/core/v1",
		"k8s.io/api/networking/v1",
		"k8s.io/api/policy/v1",
		"k8s.io/api/rbac/v1",
		"k8s.io/api/storage/v1",
		"k8s.io/apimachinery/pkg/apis/meta/v1",
		"k8s.io/apimachinery/pkg/api/resource",
		"k8s.io/apimachinery/pkg/util/intstr",
		"sigs.k8s.io/gateway-api/apis/v1",
		"github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1",
	}
}

func main() {
	outFlag := flag.String("out", "", "output file path")
	pkgFlag := flag.String("package", "assert", "package name for the generated file")
	packagesFlag := flag.String("packages", "",
		"comma-separated list of Go packages to scan; uses the default core-K8s list when empty")
	enginePkgFlag := flag.String("engine-pkg", "",
		"import path of the wilhelm assert engine when generating into a subpackage "+
			"(empty = emit bare Opt/Assertable identifiers; the core assert package itself)")
	flag.Parse()

	if *outFlag == "" {
		log.Fatal("-out flag is required")
	}

	scanned := defaultPackages()
	if *packagesFlag != "" {
		scanned = splitAndTrim(*packagesFlag)
	}

	pkgs, err := loadPackages(scanned)
	if err != nil {
		log.Fatalf("loading packages: %v", err)
	}

	knownStructs, orderedKeys := collectStructs(pkgs)

	rendered, err := emitFile(*pkgFlag, *enginePkgFlag, knownStructs, orderedKeys)
	if err != nil {
		log.Fatalf("rendering: %v", err)
	}

	formatted, err := format.Source([]byte(rendered))
	if err != nil {
		_ = os.WriteFile(*outFlag+".raw", []byte(rendered), rawDumpFilePerm)
		log.Fatalf("gofmt: %v", err)
	}

	if writeErr := os.WriteFile(*outFlag, formatted, generatedFilePerm); writeErr != nil {
		log.Fatalf("writing output: %v", writeErr)
	}
	log.Printf("generated %s (%d bytes)", *outFlag, len(formatted))
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func loadPackages(paths []string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedName |
			packages.NeedSyntax | packages.NeedTypesInfo,
	}
	return packages.Load(cfg, paths...)
}

// structInfo carries the introspected metadata for one source struct that
// has a generated assertion counterpart.
type structInfo struct {
	named      *types.Named
	struc      *types.Struct
	pkg        *packages.Package
	assertName string
}

// collectStructs walks every exported struct in pkgs and returns a map of
// fully-qualified-name → structInfo plus a sorted ordering of those keys.
// Struct names that collide across packages are disambiguated using a
// prefix derived from the package path.
func collectStructs(pkgs []*packages.Package) (map[string]*structInfo, []string) {
	nameCount := countStructNames(pkgs)
	known := map[string]*structInfo{}
	ordered := make([]string, 0)
	for _, pkg := range pkgs {
		if pkg.Types == nil {
			continue
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			named, struc, ok := exportedNamedStruct(scope.Lookup(name))
			if !ok {
				continue
			}
			key := pkg.PkgPath + "." + name
			assertName := name + "Assertion"
			if nameCount[name] > 1 {
				assertName = pkgPrefix(pkg.PkgPath) + name + "Assertion"
			}
			known[key] = &structInfo{
				named:      named,
				struc:      struc,
				pkg:        pkg,
				assertName: assertName,
			}
			ordered = append(ordered, key)
		}
	}
	slices.Sort(ordered)
	return known, ordered
}

// countStructNames counts how many packages define each exported struct
// name, so the second pass can disambiguate collisions.
func countStructNames(pkgs []*packages.Package) map[string]int {
	count := map[string]int{}
	for _, pkg := range pkgs {
		if pkg.Types == nil {
			continue
		}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			if _, _, ok := exportedNamedStruct(scope.Lookup(name)); ok {
				count[name]++
			}
		}
	}
	return count
}

// exportedNamedStruct returns the underlying *types.Named and *types.Struct
// for obj when obj is an exported, non-List, non-Status named struct type.
func exportedNamedStruct(obj types.Object) (*types.Named, *types.Struct, bool) {
	if obj == nil || !obj.Exported() {
		return nil, nil, false
	}
	named, ok := obj.Type().(*types.Named)
	if !ok {
		return nil, nil, false
	}
	struc, ok := named.Underlying().(*types.Struct)
	if !ok {
		return nil, nil, false
	}
	name := obj.Name()
	if strings.HasSuffix(name, "List") || strings.HasSuffix(name, "Status") {
		return nil, nil, false
	}
	return named, struc, true
}

// emitFile renders the generated package as Go source. enginePkg, when
// non-empty, is the import path of the wilhelm assert engine — generated
// files in subpackages qualify Opt[T] as <engine>.Opt[T].
func emitFile(
	pkgName, enginePkg string,
	knownStructs map[string]*structInfo,
	orderedKeys []string,
) (string, error) {
	f := jen.NewFile(pkgName)
	f.HeaderComment("Code generated by assertgen. DO NOT EDIT.")
	for _, key := range orderedKeys {
		info := knownStructs[key]
		emitAssertionStruct(f, enginePkg, info, knownStructs)
	}
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		return "", fmt.Errorf("render: %w", err)
	}
	return buf.String(), nil
}

// optType returns a jennifer reference to Opt — bare when generating into
// the engine package itself, otherwise qualified via the engine package.
func optType(enginePkg string) *jen.Statement {
	if enginePkg == "" {
		return jen.Id("Opt")
	}
	return jen.Qual(enginePkg, "Opt")
}

// emitAssertionStruct emits a single XxxAssertion struct + its
// IsAssertable method for one source struct.
func emitAssertionStruct(
	f *jen.File,
	enginePkg string,
	info *structInfo,
	known map[string]*structInfo,
) {
	fields := make([]jen.Code, 0, info.struc.NumFields())
	for field := range info.struc.Fields() {
		if !field.Exported() {
			continue
		}
		fields = append(fields, assertionField(enginePkg, field, known))
	}
	f.Commentf("%s is the assertion struct for %s.", info.assertName, info.named.Obj().Name())
	f.Type().Id(info.assertName).Struct(fields...)
	f.Commentf("IsAssertable marks %s as an Assertable.", info.assertName)
	f.Func().Params(jen.Id("_").Id(info.assertName)).Id("IsAssertable").Params().Block()
	f.Line()
}

// assertionField produces the jennifer code for one field of an assertion
// struct based on the corresponding source field's type.
func assertionField(enginePkg string, field *types.Var, known map[string]*structInfo) jen.Code {
	fieldType := field.Type()
	if assertName, ok := isKnownStruct(fieldType, known); ok {
		return jen.Id(field.Name()).Id(assertName)
	}
	if assertName, ok := isSliceOfKnownStruct(fieldType, known); ok {
		return jen.Id(field.Name()).Add(optType(enginePkg)).Types(jen.Index().Id(assertName))
	}
	return jen.Id(field.Name()).Add(optType(enginePkg)).Types(jenType(fieldType))
}

// isKnownStruct reports whether t (or its pointee) names a struct we are
// generating an assertion for, returning that assertion's name.
func isKnownStruct(t types.Type, known map[string]*structInfo) (string, bool) {
	t = deref(t)
	named, ok := t.(*types.Named)
	if !ok {
		return "", false
	}
	key := named.Obj().Pkg().Path() + "." + named.Obj().Name()
	info, ok := known[key]
	if !ok {
		return "", false
	}
	return info.assertName, true
}

// isSliceOfKnownStruct reports whether t is a slice whose element type is a
// known struct, returning the element's assertion name.
func isSliceOfKnownStruct(t types.Type, known map[string]*structInfo) (string, bool) {
	sl, ok := t.(*types.Slice)
	if !ok {
		return "", false
	}
	return isKnownStruct(sl.Elem(), known)
}

// pkgPrefix derives a disambiguation prefix from a package path, e.g.
// "k8s.io/apimachinery/pkg/runtime/schema" → "Schema" and
// "k8s.io/apimachinery/pkg/apis/meta/v1" → "Meta".
func pkgPrefix(pkgPath string) string {
	parts := strings.Split(pkgPath, "/")
	for _, seg := range slices.Backward(parts) {
		if seg == "" {
			continue
		}
		if isVersionSegment(seg) {
			continue
		}
		return strings.ToUpper(seg[:1]) + seg[1:]
	}
	return ""
}

// isVersionSegment reports whether seg looks like a K8s API version
// segment (v1, v2, v1beta1, ...).
func isVersionSegment(seg string) bool {
	if len(seg) < 2 || seg[0] != 'v' {
		return false
	}
	return seg[1] >= '0' && seg[1] <= '9'
}

// deref strips pointer indirection from a type.
func deref(t types.Type) types.Type {
	for {
		p, ok := t.(*types.Pointer)
		if !ok {
			return t
		}
		t = p.Elem()
	}
}

// jenType converts a go/types.Type to a jennifer code statement.
func jenType(t types.Type) *jen.Statement {
	switch tt := t.(type) {
	case *types.Named:
		return namedQual(tt.Obj())
	case *types.Alias:
		return namedQual(tt.Obj())
	case *types.Pointer:
		return jen.Op("*").Add(jenType(tt.Elem()))
	case *types.Slice:
		return jen.Index().Add(jenType(tt.Elem()))
	case *types.Map:
		return jen.Map(jenType(tt.Key())).Add(jenType(tt.Elem()))
	case *types.Basic:
		return jen.Id(tt.Name())
	case *types.Interface:
		if tt.NumMethods() == 0 {
			return jen.Any()
		}
		return jen.Id(tt.String())
	case *types.Array:
		return jen.Index(jen.Lit(int(tt.Len()))).Add(jenType(tt.Elem()))
	case *types.Struct:
		// Anonymous struct — fall back to any.
		return jen.Any()
	default:
		return jen.Id(t.String())
	}
}

func namedQual(obj types.Object) *jen.Statement {
	pkg := obj.Pkg()
	if pkg == nil {
		return jen.Id(obj.Name())
	}
	return jen.Qual(pkg.Path(), obj.Name())
}
