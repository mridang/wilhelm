// Package anchors holds blank imports for every package that wilhelm's
// generators introspect. The imports keep the modules in `go.mod` and
// available to golang.org/x/tools/go/packages.Load before the generated
// files exist (the chicken-and-egg bootstrap problem). The package is
// otherwise unused at runtime.
package anchors

// Blank imports anchor each package in go.mod so packages.Load can find
// them during code generation.
import (
	_ "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1" // generator target
	_ "k8s.io/api/apps/v1"                                                        // generator target
	_ "k8s.io/api/autoscaling/v2"                                                 // generator target
	_ "k8s.io/api/batch/v1"                                                       // generator target
	_ "k8s.io/api/core/v1"                                                        // generator target
	_ "k8s.io/api/networking/v1"                                                  // generator target
	_ "k8s.io/api/policy/v1"                                                      // generator target
	_ "k8s.io/api/rbac/v1"                                                        // generator target
	_ "k8s.io/api/storage/v1"                                                     // generator target
	_ "k8s.io/apimachinery/pkg/api/resource"                                      // generator target
	_ "k8s.io/apimachinery/pkg/apis/meta/v1"                                      // generator target
	_ "k8s.io/apimachinery/pkg/util/intstr"                                       // generator target
	_ "k8s.io/client-go/kubernetes"                                               // envgen target
	_ "sigs.k8s.io/gateway-api/apis/v1"                                           // generator target
)
