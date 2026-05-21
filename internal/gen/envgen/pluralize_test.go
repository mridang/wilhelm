package main

import "testing"

const (
	kindPolicy     = "Policy"
	pluralPolicies = "policies"
)

func TestPluralize(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind      string
		overrides map[string]string
		want      string
	}{
		// Default consonant-y → ies.
		{kind: kindPolicy, want: pluralPolicies},
		{kind: "Repository", want: "repositories"},

		// Vowel-y keeps -ys.
		{kind: "Gateway", want: "gateways"},
		{kind: "Key", want: "keys"},

		// s|x|z|ch|sh → es.
		{kind: "Service", want: "services"}, // default branch
		{kind: "Class", want: "classes"},
		{kind: "Box", want: "boxes"},
		{kind: "Buzz", want: "buzzes"},
		{kind: "Branch", want: "branches"},
		{kind: "Brush", want: "brushes"},

		// Built-in irregulars.
		{kind: "Endpoints", want: "endpoints"},
		{kind: "StorageClass", want: "storageclasses"},
		{kind: "PriorityClass", want: "priorityclasses"},
		{kind: "RuntimeClass", want: "runtimeclasses"},

		// Default branch — plain +s.
		{kind: "Application", want: "applications"},
		{kind: "AppProject", want: "appprojects"},
		{kind: "Certificate", want: "certificates"},
		{kind: "ExternalSecret", want: "externalsecrets"},

		// Overrides win.
		{
			kind:      kindPolicy,
			overrides: map[string]string{kindPolicy: "policy-overridden"},
			want:      "policy-overridden",
		},
	}
	for _, tc := range cases {
		got := pluralize(tc.kind, tc.overrides)
		if got != tc.want {
			t.Errorf("pluralize(%q, %v) = %q; want %q", tc.kind, tc.overrides, got, tc.want)
		}
	}
}

func TestEndsInVowelY(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"gateway":    true,
		"key":        true,
		"valley":     true,
		"alloy":      true,
		"buoy":       true,
		"policy":     false,
		"repository": false,
		"family":     false,
		"y":          false,
		"":           false,
		"by":         false,
	}
	for in, want := range cases {
		got := endsInVowelY(in)
		if got != want {
			t.Errorf("endsInVowelY(%q) = %v; want %v", in, got, want)
		}
	}
}

func TestParsePluralOverrides(t *testing.T) {
	t.Parallel()
	got := parsePluralOverrides("Policy:policies, Foo : foos , Bar:bars")
	want := map[string]string{
		"Policy": "policies",
		"Foo":    "foos",
		"Bar":    "bars",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("override %q = %q; want %q", k, got[k], v)
		}
	}
}
