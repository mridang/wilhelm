package main

import "strings"

// builtinPlurals catches the common irregular K8s/CRD plurals so callers
// don't have to supply overrides for them.
//
//nolint:gochecknoglobals // table of irregular plurals
var builtinPlurals = map[string]string{
	"Endpoints":        "endpoints",
	"Policy":           "policies",
	"NetworkPolicy":    "networkpolicies",
	"PodPolicy":        "podpolicies",
	"PriorityClass":    "priorityclasses",
	"StorageClass":     "storageclasses",
	"IngressClass":     "ingressclasses",
	"RuntimeClass":     "runtimeclasses",
	"VolumeAttachment": "volumeattachments",
}

// pluralize lowercases kind and applies English plural-s rules with a
// small carve-out for irregular forms. overrides win, then builtinPlurals,
// then the default rules:
//
//   - ends in y preceded by a consonant   → -y +ies   (Policy   → policies)
//   - ends in s|x|z|ch|sh                  → +es        (Class    → classes)
//   - otherwise                            → +s         (Service  → services)
func pluralize(kind string, overrides map[string]string) string {
	if kind == "" {
		return ""
	}
	if p, ok := overrides[kind]; ok {
		return p
	}
	if p, ok := builtinPlurals[kind]; ok {
		return p
	}
	lower := strings.ToLower(kind)
	switch {
	case strings.HasSuffix(lower, "y") && !endsInVowelY(lower):
		return lower[:len(lower)-1] + "ies"
	case strings.HasSuffix(lower, "s"),
		strings.HasSuffix(lower, "x"),
		strings.HasSuffix(lower, "z"),
		strings.HasSuffix(lower, "ch"),
		strings.HasSuffix(lower, "sh"):
		return lower + "es"
	default:
		return lower + "s"
	}
}

// endsInVowelY reports whether s ends in a vowel followed by 'y'
// (e.g. "valley", "key") — those keep -ys, not -ies.
func endsInVowelY(s string) bool {
	if len(s) < 2 || s[len(s)-1] != 'y' {
		return false
	}
	switch s[len(s)-2] {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	default:
		return false
	}
}
