// Command envgen has two modes:
//
//   - clientset (default): generate typed getter methods on *env.Env for
//     every resource the upstream kubernetes.Interface exposes, plus
//     type-switching AssertPartial / AssertNone dispatchers.
//
//   - crd: generate dynamic-client-based getter functions for every
//     top-level Kind in a list of CRD type packages, plus type-switching
//     AssertPartial / AssertNone dispatchers. Top-level Kinds are
//     identified by structs that embed both metav1.TypeMeta and
//     metav1.ObjectMeta.
package main

import (
	"errors"
	"flag"
	"go/format"
	"log"
	"os"
	"strings"
)

const (
	generatedFilePerm = 0o600
	rawDumpFilePerm   = 0o600
)

func main() {
	mode := flag.String("mode", "clientset", "generator mode: 'clientset' or 'crd'")
	outFlag := flag.String("out", "", "output file path")
	pkgFlag := flag.String("package", "env", "package name for the generated file")

	// CRD-mode flags (ignored in clientset mode).
	groupFlag := flag.String("group", "",
		"crd mode only: API group (e.g. 'argoproj.io')")
	packagesFlag := flag.String("packages", "",
		"crd mode only: comma-separated Go packages to scan for top-level Kinds")
	assertPkgFlag := flag.String("assert-pkg", "",
		"crd mode only: import path of the matching assert subpackage")
	clusterScopedFlag := flag.String("cluster-scoped", "",
		"crd mode only: comma-separated Kinds that are cluster-scoped (default: namespaced)")
	pluralsFlag := flag.String("plurals", "",
		"crd mode only: comma-separated Kind:plural overrides (e.g. Policy:policies)")

	flag.Parse()

	if *outFlag == "" {
		log.Fatal("-out flag is required")
	}

	var (
		rendered string
		summary  string
		err      error
	)
	switch *mode {
	case "clientset":
		rendered, summary, err = runClientsetMode(*pkgFlag)
	case "crd":
		rendered, summary, err = runCRDMode(crdConfig{
			pkgName:        *pkgFlag,
			group:          *groupFlag,
			packages:       splitAndTrim(*packagesFlag),
			assertPkg:      *assertPkgFlag,
			clusterScoped:  newStringSet(splitAndTrim(*clusterScopedFlag)),
			pluralOverride: parsePluralOverrides(*pluralsFlag),
		})
	default:
		log.Fatalf("unknown -mode %q (want 'clientset' or 'crd')", *mode)
	}
	if err != nil {
		log.Fatalf("generating: %v", err)
	}

	formatted, fmtErr := format.Source([]byte(rendered))
	if fmtErr != nil {
		_ = os.WriteFile(*outFlag+".raw", []byte(rendered), rawDumpFilePerm)
		log.Fatalf("gofmt: %v", fmtErr)
	}

	if writeErr := os.WriteFile(*outFlag, formatted, generatedFilePerm); writeErr != nil {
		log.Fatalf("writing output: %v", writeErr)
	}
	log.Printf("generated %s (%d bytes); %s", *outFlag, len(formatted), summary)
}

// splitAndTrim splits a comma-separated value, trimming each element and
// dropping empty results.
func splitAndTrim(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// parsePluralOverrides parses "Kind:plural,Kind:plural" into a map.
func parsePluralOverrides(s string) map[string]string {
	out := map[string]string{}
	for _, pair := range splitAndTrim(s) {
		k, v, ok := strings.Cut(pair, ":")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

// stringSet is a tiny lookup helper.
type stringSet map[string]struct{}

func newStringSet(in []string) stringSet {
	s := stringSet{}
	for _, v := range in {
		s[v] = struct{}{}
	}
	return s
}

func (s stringSet) has(v string) bool {
	_, ok := s[v]
	return ok
}

// errPackagesRequired is returned when CRD mode is invoked without
// -packages.
var errPackagesRequired = errors.New("-packages is required in crd mode")
