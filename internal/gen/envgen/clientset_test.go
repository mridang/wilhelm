package main

import "testing"

func TestIsAlphaAPI(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"k8s.io/api/scheduling/v1alpha2":   true,
		"k8s.io/api/node/v1alpha1":         true,
		"k8s.io/api/resource/v1beta2":      false,
		"k8s.io/api/apps/v1":               false,
		"k8s.io/api/alphabet/v1":           false,
		"k8s.io/api/autoscaling/v2":        false,
		"k8s.io/api/coordination/v1alpha2": true,
	}
	for pkg, want := range cases {
		if got := isAlphaAPI(pkg); got != want {
			t.Errorf("isAlphaAPI(%q) = %v, want %v", pkg, got, want)
		}
	}
}
