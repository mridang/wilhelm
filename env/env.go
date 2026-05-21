package env

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	errNilConfig      = errors.New("nil *rest.Config")
	errEmptyNamespace = errors.New("empty namespace")
)

// Env is the per-test environment passed to wilhelm's resource getters and
// AssertPartial dispatcher. It owns a Kubernetes typed client, a dynamic
// client (for CRDs that wilhelm does not have a typed clientset for), the
// target namespace, and a base context used by every API call.
type Env struct {
	// Ctx is forwarded to every API call. Tests may set a per-test timeout
	// or cancellation context by replacing this field.
	Ctx context.Context

	// Namespace is the target namespace for every namespaced lookup.
	Namespace string

	// Client is the typed Kubernetes clientset.
	Client *kubernetes.Clientset

	// DynamicClient fetches arbitrary resources (CRDs and core types alike)
	// as unstructured.Unstructured values.
	DynamicClient dynamic.Interface
}

// NewEnv builds an Env from a *rest.Config and a target namespace. Tests
// obtain the rest.Config however they like (in-cluster, kubeconfig file,
// custom transport) and pass it in.
func NewEnv(cfg *rest.Config, namespace string) (*Env, error) {
	return NewEnvWithContext(context.Background(), cfg, namespace)
}

// NewEnvWithContext is like NewEnv but accepts an explicit base context.
func NewEnvWithContext(ctx context.Context, cfg *rest.Config, namespace string) (*Env, error) {
	if cfg == nil {
		return nil, errNilConfig
	}
	if namespace == "" {
		return nil, errEmptyNamespace
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kubernetes.NewForConfig: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("dynamic.NewForConfig: %w", err)
	}
	return &Env{
		Ctx:           ctx,
		Namespace:     namespace,
		Client:        client,
		DynamicClient: dyn,
	}, nil
}
