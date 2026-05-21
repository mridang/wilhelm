package main

import (
	"cmp"
	"errors"
	"fmt"
	"go/types"
	"os"
	"slices"
	"strings"

	"github.com/dave/jennifer/jen"
	"golang.org/x/tools/go/packages"
)

const (
	priorityAssertion = 1000
	priorityStable    = 200
	priorityBeta      = 100
	priorityAlpha     = 0
	decimalBase       = 10
)

var (
	errNoPackages   = errors.New("no packages loaded for k8s.io/client-go/kubernetes")
	errPkgErrors    = errors.New("k8s.io/client-go/kubernetes package had errors")
	errIfaceMissing = errors.New("kubernetes.Interface not found in scope")
	errNotInterface = errors.New("kubernetes.Interface is not an interface type")
)

const (
	wilhelmAssertPkg = "github.com/mridang/wilhelm/assert"
	requirePkg       = "github.com/stretchr/testify/require"
	metav1Pkg        = "k8s.io/apimachinery/pkg/apis/meta/v1"
	apierrorsPkg     = "k8s.io/apimachinery/pkg/api/errors"
)

// assertgenPackages mirrors the set in assertgen's defaultPackages. Only
// resources whose Go type comes from one of these packages gets dispatched
// through AssertPartial.
func assertgenPackages() map[string]bool {
	return map[string]bool{
		"k8s.io/api/apps/v1":        true,
		"k8s.io/api/autoscaling/v2": true,
		"k8s.io/api/batch/v1":       true,
		"k8s.io/api/core/v1":        true,
		"k8s.io/api/networking/v1":  true,
		"k8s.io/api/policy/v1":      true,
		"k8s.io/api/rbac/v1":        true,
		"k8s.io/api/storage/v1":     true,
	}
}

// resource captures one Get-able K8s API resource discovered from the
// kubernetes.Interface method graph.
type resource struct {
	Name         string // possibly disambiguated method name suffix
	TypeName     string // original Go type name in the package
	TypePkgPath  string // import path of the type
	GroupMethod  string // e.g. "AppsV1"
	Plural       string // e.g. "Deployments" (the method name on the group interface)
	Namespaced   bool
	HasAssertion bool
}

// AssertName returns the corresponding assertion struct name in the
// wilhelm assert package.
func (r resource) AssertName() string { return r.Name + "Assertion" }

// configKinds pairs a crdConfig with the top-level Kinds discovered from
// its packages. Used by clientset mode when -crd flags are present.
type configKinds struct {
	cfg   crdConfig
	kinds []kind
}

// runClientsetMode is the entrypoint for the default clientset-introspection
// generator. crds is an optional list of CRD package configs whose Kinds
// get additional method getters on *Env and dispatch entries in
// AssertPartial/AssertNone.
func runClientsetMode(pkgName string, crds []crdConfig) (string, string, error) {
	resources, err := discoverResources()
	if err != nil {
		return "", "", fmt.Errorf("discovering resources: %w", err)
	}
	disambiguateNames(resources)
	slices.SortFunc(resources, func(a, b resource) int {
		return cmp.Or(
			cmp.Compare(a.GroupMethod, b.GroupMethod),
			cmp.Compare(a.Name, b.Name),
		)
	})

	allCRDKinds := make([]configKinds, 0, len(crds))
	for _, cfg := range crds {
		kinds, crdErr := discoverKinds(cfg)
		if crdErr != nil {
			return "", "", fmt.Errorf("discovering CRD kinds for group %s: %w", cfg.group, crdErr)
		}
		slices.SortFunc(kinds, func(a, b kind) int {
			return cmp.Or(
				cmp.Compare(a.Version, b.Version),
				cmp.Compare(a.TypeName, b.TypeName),
			)
		})
		allCRDKinds = append(allCRDKinds, configKinds{cfg: cfg, kinds: kinds})
	}

	rendered, err := emitClientsetFile(pkgName, resources, allCRDKinds)
	if err != nil {
		return "", "", fmt.Errorf("rendering: %w", err)
	}
	withAssert := 0
	for _, r := range resources {
		if r.HasAssertion {
			withAssert++
		}
	}
	crdTotal := 0
	for _, ck := range allCRDKinds {
		crdTotal += len(ck.kinds)
	}
	summary := fmt.Sprintf("clientset mode: %d resources (%d with assertions), %d CRD kinds",
		len(resources), withAssert, crdTotal)
	return rendered, summary, nil
}

// discoverResources walks kubernetes.Interface and returns every Get-able
// resource we can address through it.
func discoverResources() ([]resource, error) {
	cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedName}
	pkgs, err := packages.Load(cfg, "k8s.io/client-go/kubernetes")
	if err != nil {
		return nil, fmt.Errorf("packages.Load: %w", err)
	}
	if len(pkgs) == 0 {
		return nil, errNoPackages
	}
	for _, e := range pkgs[0].Errors {
		fmt.Fprintf(os.Stderr, "envgen: package error: %s\n", e.Error())
	}
	if len(pkgs[0].Errors) > 0 {
		return nil, errPkgErrors
	}
	ifaceObj := pkgs[0].Types.Scope().Lookup("Interface")
	if ifaceObj == nil {
		return nil, errIfaceMissing
	}
	ifaceType, isIface := ifaceObj.Type().Underlying().(*types.Interface)
	if !isIface {
		return nil, errNotInterface
	}
	assertPkgs := assertgenPackages()
	var out []resource
	for method := range ifaceType.Methods() {
		out = append(out, resourcesForGroup(method, assertPkgs)...)
	}
	return out, nil
}

// resourcesForGroup returns the resources reachable through one group
// method on kubernetes.Interface (e.g. AppsV1, CoreV1).
func resourcesForGroup(groupMethod *types.Func, assertPkgs map[string]bool) []resource {
	if groupMethod.Name() == "Discovery" {
		return nil
	}
	groupSig, isSig := groupMethod.Type().(*types.Signature)
	if !isSig || groupSig.Results().Len() != 1 {
		return nil
	}
	groupRetType := groupSig.Results().At(0).Type()
	if _, isIface := groupRetType.Underlying().(*types.Interface); !isIface {
		return nil
	}
	mset := types.NewMethodSet(groupRetType)
	out := make([]resource, 0, mset.Len())
	for sel := range mset.Methods() {
		if r, ok := resourceForGetter(sel.Obj(), groupMethod.Name(), assertPkgs); ok {
			out = append(out, r)
		}
	}
	return out
}

// resourceForGetter inspects one resource-getter method on a group
// interface (e.g. AppsV1Interface.Deployments) and returns the matching
// resource description.
func resourceForGetter(
	obj types.Object,
	groupName string,
	assertPkgs map[string]bool,
) (resource, bool) {
	fn, ok := obj.(*types.Func)
	if !ok || fn.Name() == "RESTClient" {
		return resource{}, false
	}
	fnSig, ok := fn.Type().(*types.Signature)
	if !ok || fnSig.Results().Len() != 1 {
		return resource{}, false
	}
	namespaced := fnSig.Params().Len() == 1
	resRetType := fnSig.Results().At(0).Type()
	typeName, typePkgPath, ok := typeFromGetMethod(resRetType)
	if !ok {
		return resource{}, false
	}
	hasAssertion := assertPkgs[typePkgPath] &&
		!strings.HasSuffix(typeName, "List") &&
		!strings.HasSuffix(typeName, "Status")
	return resource{
		Name:         typeName,
		TypeName:     typeName,
		TypePkgPath:  typePkgPath,
		GroupMethod:  groupName,
		Plural:       fn.Name(),
		Namespaced:   namespaced,
		HasAssertion: hasAssertion,
	}, true
}

// typeFromGetMethod finds the Get() method on a resource interface and
// returns (typeName, typePkgPath, true) for the pointee type of its first
// return value, or zeros and false if the interface is shaped differently.
func typeFromGetMethod(resourceIfaceType types.Type) (string, string, bool) {
	var getMethod *types.Func
	for sel := range types.NewMethodSet(resourceIfaceType).Methods() {
		m, isFunc := sel.Obj().(*types.Func)
		if !isFunc || m.Name() != "Get" {
			continue
		}
		getMethod = m
		break
	}
	if getMethod == nil {
		return "", "", false
	}
	getSig, isSig := getMethod.Type().(*types.Signature)
	if !isSig || getSig.Results().Len() < 1 {
		return "", "", false
	}
	ptr, isPtr := getSig.Results().At(0).Type().(*types.Pointer)
	if !isPtr {
		return "", "", false
	}
	named, isNamed := ptr.Elem().(*types.Named)
	if !isNamed {
		return "", "", false
	}
	typePkg := named.Obj().Pkg()
	if typePkg == nil {
		return "", "", false
	}
	return named.Obj().Name(), typePkg.Path(), true
}

// disambiguateNames resolves method-name collisions across API group
// versions by keeping the highest-priority resource at the short name and
// prefixing all others with their GroupMethod.
func disambiguateNames(resources []resource) {
	nameToIdxs := make(map[string][]int)
	for i := range resources {
		nameToIdxs[resources[i].Name] = append(nameToIdxs[resources[i].Name], i)
	}
	for _, idxs := range nameToIdxs {
		if len(idxs) <= 1 {
			continue
		}
		bestIdx := idxs[0]
		for _, idx := range idxs[1:] {
			if resourcePriority(resources[idx]) > resourcePriority(resources[bestIdx]) {
				bestIdx = idx
			}
		}
		for _, idx := range idxs {
			if idx == bestIdx {
				continue
			}
			resources[idx].Name = resources[idx].GroupMethod + resources[idx].Name
		}
	}
}

// resourcePriority computes a higher-is-better score used to pick the
// canonical resource for a colliding type name. Stable > beta > alpha;
// resources with assertions outrank everything; higher version is the
// tiebreaker.
func resourcePriority(r resource) int {
	p := 0
	if r.HasAssertion {
		p += priorityAssertion
	}
	switch {
	case strings.Contains(r.TypePkgPath, "alpha"):
		p += priorityAlpha
	case strings.Contains(r.TypePkgPath, "beta"):
		p += priorityBeta
	default:
		p += priorityStable
	}
	p += extractVersion(r.TypePkgPath)
	return p
}

// extractVersion extracts the leading numeric version from the final path
// segment, e.g. "k8s.io/api/autoscaling/v2" → 2.
func extractVersion(pkgPath string) int {
	parts := strings.Split(pkgPath, "/")
	last := parts[len(parts)-1]
	v := 0
	for _, c := range last {
		if c >= '0' && c <= '9' {
			v = v*decimalBase + int(c-'0')
			continue
		}
		if v > 0 {
			break
		}
	}
	return v
}

// emitClientsetFile renders the generated env package source for the
// clientset-introspection mode. allCRDKinds holds optional CRD extension
// kinds whose getters are emitted as methods on *Env alongside the
// clientset getters.
func emitClientsetFile(pkgName string, resources []resource, allCRDKinds []configKinds) (string, error) {
	f := jen.NewFile(pkgName)
	f.HeaderComment("Code generated by envgen. DO NOT EDIT.")
	for _, r := range resources {
		emitClientsetGetter(f, r)
		emitClientsetGetterE(f, r)
	}
	// Emit CRD method getters (dynamic-client based) alongside clientset getters.
	for _, ck := range allCRDKinds {
		for _, k := range ck.kinds {
			emitCRDMethodGetter(f, ck.cfg, k)
			emitCRDMethodGetterE(f, ck.cfg, k)
		}
	}
	assertResources := filterWithAssertion(resources)
	emitClientsetAssertPartial(f, assertResources, allCRDKinds)
	emitClientsetAssertNone(f, assertResources, allCRDKinds)
	var buf strings.Builder
	if err := f.Render(&buf); err != nil {
		return "", fmt.Errorf("render: %w", err)
	}
	return buf.String(), nil
}

func filterWithAssertion(resources []resource) []resource {
	out := make([]resource, 0, len(resources))
	for _, r := range resources {
		if r.HasAssertion {
			out = append(out, r)
		}
	}
	return out
}

// emitClientsetGetter emits Get<Resource>(t, name) *pkg.Type.
func emitClientsetGetter(f *jen.File, r resource) {
	f.Commentf("Get%s fetches a %s by name, failing the test on error.", r.Name, r.Name)
	f.Func().Params(jen.Id("env").Op("*").Id("Env")).Id("Get"+r.Name).Params(
		jen.Id("t").Op("*").Qual("testing", "T"),
		jen.Id("name").String(),
	).Op("*").Qual(r.TypePkgPath, r.TypeName).Block(
		jen.Id("t").Dot("Helper").Call(),
		jen.List(jen.Id("obj"), jen.Id("err")).Op(":=").Add(clientsetGetExpr(r)),
		jen.Qual(requirePkg, "NoError").Call(
			jen.Id("t"), jen.Id("err"),
			jen.Lit(fmt.Sprintf("failed to get %s %%s", r.Name)), jen.Id("name"),
		),
		jen.Return(jen.Id("obj")),
	)
	f.Line()
}

// emitClientsetGetterE emits Get<Resource>E(t, name) (*pkg.Type, error).
func emitClientsetGetterE(f *jen.File, r resource) {
	f.Commentf("Get%sE fetches a %s by name, returning the error for non-existence checks.", r.Name, r.Name)
	f.Func().Params(jen.Id("env").Op("*").Id("Env")).Id("Get"+r.Name+"E").Params(
		jen.Id("t").Op("*").Qual("testing", "T"),
		jen.Id("name").String(),
	).Parens(jen.List(
		jen.Op("*").Qual(r.TypePkgPath, r.TypeName),
		jen.Error(),
	)).Block(
		jen.Id("t").Dot("Helper").Call(),
		jen.Return(clientsetGetExpr(r)),
	)
	f.Line()
}

// clientsetGetExpr builds env.Client.<Group>().<Plural>([env.Namespace]).Get(env.Ctx, name, metav1.GetOptions{}).
func clientsetGetExpr(r resource) *jen.Statement {
	chain := jen.Id("env").Dot("Client").Dot(r.GroupMethod).Call()
	if r.Namespaced {
		chain = chain.Dot(r.Plural).Call(jen.Id("env").Dot("Namespace"))
	} else {
		chain = chain.Dot(r.Plural).Call()
	}
	return chain.Dot("Get").Call(
		jen.Id("env").Dot("Ctx"),
		jen.Id("name"),
		jen.Qual(metav1Pkg, "GetOptions").Values(),
	)
}

// emitClientsetAssertPartial emits the (*Env).AssertPartial method that
// type-switches on the concrete assertion struct, fetches the matching
// resource, and runs assert.Partial. allCRDKinds adds additional cases
// for CRD-extended types.
func emitClientsetAssertPartial(f *jen.File, resources []resource, allCRDKinds []configKinds) {
	cases := make([]jen.Code, 0, len(resources)*2+1)
	for _, r := range resources {
		cases = append(cases,
			jen.Case(jen.Qual(wilhelmAssertPkg, r.AssertName())).Block(
				jen.Qual(wilhelmAssertPkg, "Partial").Call(
					jen.Id("t"),
					jen.Id("env").Dot("Get"+r.Name).Call(jen.Id("t"), jen.Id("name")),
					jen.Id("a"),
					jen.Id("name"),
				),
			),
			jen.Case(jen.Op("*").Qual(wilhelmAssertPkg, r.AssertName())).Block(
				jen.Qual(wilhelmAssertPkg, "Partial").Call(
					jen.Id("t"),
					jen.Id("env").Dot("Get"+r.Name).Call(jen.Id("t"), jen.Id("name")),
					jen.Op("*").Id("a"),
					jen.Id("name"),
				),
			),
		)
	}
	// CRD-extended kinds (gateway-api, prometheus, …).
	for _, ck := range allCRDKinds {
		assertPkg := ck.cfg.assertPkg
		if assertPkg == "" {
			assertPkg = wilhelmAssertPkg
		}
		for _, k := range ck.kinds {
			cases = append(cases,
				jen.Case(jen.Qual(assertPkg, k.AssertName())).Block(
					jen.Qual(wilhelmAssertPkg, "Partial").Call(
						jen.Id("t"),
						jen.Id("env").Dot("Get"+k.TypeName).Call(jen.Id("t"), jen.Id("name")),
						jen.Id("a"),
						jen.Id("name"),
					),
				),
				jen.Case(jen.Op("*").Qual(assertPkg, k.AssertName())).Block(
					jen.Qual(wilhelmAssertPkg, "Partial").Call(
						jen.Id("t"),
						jen.Id("env").Dot("Get"+k.TypeName).Call(jen.Id("t"), jen.Id("name")),
						jen.Op("*").Id("a"),
						jen.Id("name"),
					),
				),
			)
		}
	}
	cases = append(cases,
		jen.Default().Block(
			jen.Id("t").Dot("Fatalf").Call(
				jen.Lit("env.AssertPartial: no resource registered for assertion type %T"),
				jen.Id("assertion"),
			),
		),
	)
	f.Comment("AssertPartial fetches the K8s resource implied by the assertion type and")
	f.Comment("runs a partial assertion. The resource type is inferred from the concrete")
	f.Comment("assertion struct via a type switch.")
	f.Func().Params(jen.Id("env").Op("*").Id("Env")).Id("AssertPartial").Params(
		jen.Id("t").Op("*").Qual("testing", "T"),
		jen.Id("name").String(),
		jen.Id("assertion").Qual(wilhelmAssertPkg, "Assertable"),
	).Block(
		jen.Id("t").Dot("Helper").Call(),
		jen.Switch(jen.Id("a").Op(":=").Id("assertion").Assert(jen.Type())).Block(cases...),
	)
	f.Line()
}

// emitClientsetAssertNone emits the (*Env).AssertNone method that fetches the
// resource by name and asserts the API returned NotFound. allCRDKinds adds
// additional cases for CRD-extended types.
func emitClientsetAssertNone(f *jen.File, resources []resource, allCRDKinds []configKinds) {
	cases := make([]jen.Code, 0, len(resources)+1)
	for _, r := range resources {
		cases = append(cases,
			jen.Case(
				jen.Qual(wilhelmAssertPkg, r.AssertName()),
				jen.Op("*").Qual(wilhelmAssertPkg, r.AssertName()),
			).Block(
				jen.List(jen.Id("_"), jen.Id("err")).Op(":=").
					Id("env").Dot("Get"+r.Name+"E").Call(jen.Id("t"), jen.Id("name")),
				jen.Qual(requirePkg, "True").Call(
					jen.Id("t"),
					jen.Qual(apierrorsPkg, "IsNotFound").Call(jen.Id("err")),
					jen.Lit(fmt.Sprintf("%s %%q should not exist (err: %%v)", r.Name)),
					jen.Id("name"),
					jen.Id("err"),
				),
			),
		)
	}
	// CRD-extended kinds (gateway-api, prometheus, …).
	for _, ck := range allCRDKinds {
		assertPkg := ck.cfg.assertPkg
		if assertPkg == "" {
			assertPkg = wilhelmAssertPkg
		}
		for _, k := range ck.kinds {
			cases = append(cases,
				jen.Case(
					jen.Qual(assertPkg, k.AssertName()),
					jen.Op("*").Qual(assertPkg, k.AssertName()),
				).Block(
					jen.List(jen.Id("_"), jen.Id("err")).Op(":=").
						Id("env").Dot("Get"+k.TypeName+"E").Call(jen.Id("t"), jen.Id("name")),
					jen.Qual(requirePkg, "True").Call(
						jen.Id("t"),
						jen.Qual(apierrorsPkg, "IsNotFound").Call(jen.Id("err")),
						jen.Lit(fmt.Sprintf("%s %%q should not exist (err: %%v)", k.TypeName)),
						jen.Id("name"),
						jen.Id("err"),
					),
				),
			)
		}
	}
	cases = append(cases,
		jen.Default().Block(
			jen.Id("t").Dot("Fatalf").Call(
				jen.Lit("env.AssertNone: no resource registered for assertion type %T"),
				jen.Id("assertion"),
			),
		),
	)
	f.Comment("AssertNone asserts that the K8s resource implied by the assertion type")
	f.Comment("does not exist (the GET returns NotFound). The resource type is inferred")
	f.Comment("from the concrete assertion struct via a type switch.")
	f.Func().Params(jen.Id("env").Op("*").Id("Env")).Id("AssertNone").Params(
		jen.Id("t").Op("*").Qual("testing", "T"),
		jen.Id("name").String(),
		jen.Id("assertion").Qual(wilhelmAssertPkg, "Assertable"),
	).Block(
		jen.Id("t").Dot("Helper").Call(),
		jen.Switch(jen.Id("assertion").Assert(jen.Type())).Block(cases...),
	)
}

// emitCRDMethodGetter emits (env *Env) Get<Kind>(t, name) *pkg.Kind.
// This is the method form used when generating CRD getters into the root
// env package, as opposed to the package-level function form used in
// CRD subpackages.
func emitCRDMethodGetter(f *jen.File, _ crdConfig, k kind) {
	f.Commentf(
		"Get%s fetches a %s by name via the dynamic client, failing the test on error.",
		k.TypeName, k.TypeName,
	)
	f.Func().Params(jen.Id("env").Op("*").Id("Env")).Id("Get"+k.TypeName).Params(
		jen.Id("t").Op("*").Qual("testing", "T"),
		jen.Id("name").String(),
	).Op("*").Qual(k.TypePkgPath, k.TypeName).Block(
		jen.Id("t").Dot("Helper").Call(),
		jen.List(jen.Id("obj"), jen.Id("err")).Op(":=").
			Id("env").Dot("Get"+k.TypeName+"E").Call(jen.Id("t"), jen.Id("name")),
		jen.Qual(requirePkg, "NoError").Call(
			jen.Id("t"), jen.Id("err"),
			jen.Lit(fmt.Sprintf("failed to get %s %%s", k.TypeName)), jen.Id("name"),
		),
		jen.Return(jen.Id("obj")),
	)
	f.Line()
}

// emitCRDMethodGetterE emits (env *Env) Get<Kind>E(t, name) (*pkg.Kind, error).
func emitCRDMethodGetterE(f *jen.File, cfg crdConfig, k kind) {
	gvr := jen.Qual(schemaPkg, "GroupVersionResource").Values(jen.Dict{
		jen.Id("Group"):    jen.Lit(cfg.group),
		jen.Id("Version"):  jen.Lit(k.Version),
		jen.Id("Resource"): jen.Lit(k.Plural),
	})
	var resourceChain *jen.Statement
	if k.Namespaced {
		resourceChain = jen.Id("env").Dot("DynamicClient").Dot("Resource").Call(jen.Id("gvr")).
			Dot("Namespace").Call(jen.Id("env").Dot("Namespace"))
	} else {
		resourceChain = jen.Id("env").Dot("DynamicClient").Dot("Resource").Call(jen.Id("gvr"))
	}
	f.Commentf(
		"Get%sE fetches a %s by name via the dynamic client, returning the error for non-existence checks.",
		k.TypeName, k.TypeName,
	)
	f.Func().Params(jen.Id("env").Op("*").Id("Env")).Id("Get"+k.TypeName+"E").Params(
		jen.Id("t").Op("*").Qual("testing", "T"),
		jen.Id("name").String(),
	).Parens(jen.List(
		jen.Op("*").Qual(k.TypePkgPath, k.TypeName),
		jen.Error(),
	)).Block(
		jen.Id("t").Dot("Helper").Call(),
		jen.Id("gvr").Op(":=").Add(gvr),
		jen.List(jen.Id("u"), jen.Id("err")).Op(":=").Add(resourceChain).
			Dot("Get").Call(
			jen.Id("env").Dot("Ctx"), jen.Id("name"),
			jen.Qual(metav1Pkg, "GetOptions").Values(),
		),
		jen.If(jen.Id("err").Op("!=").Nil()).Block(
			jen.Return(jen.Nil(), jen.Id("err")),
		),
		jen.Var().Id("out").Qual(k.TypePkgPath, k.TypeName),
		jen.If(jen.Id("convErr").Op(":=").Qual(runtimePkg, "DefaultUnstructuredConverter").
			Dot("FromUnstructured").Call(
			jen.Id("u").Dot("Object"), jen.Op("&").Id("out"),
		).Op(";").Id("convErr").Op("!=").Nil()).Block(
			jen.Return(jen.Nil(),
				jen.Qual("fmt", "Errorf").Call(
					jen.Lit(fmt.Sprintf("convert %s from unstructured: %%w", k.TypeName)),
					jen.Id("convErr"),
				),
			),
		),
		jen.Return(jen.Op("&").Id("out"), jen.Nil()),
	)
	f.Line()
}
