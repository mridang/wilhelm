// Package env provides the per-test wilhelm environment plus typed
// resource getters for every namespaced and cluster-scoped resource that
// the upstream kubernetes.Interface exposes, as well as dynamic-client
// getters for Gateway API and Prometheus Operator types.
//
// Re-generate the getter catalogue (zz_generated.go) with:
//
//	go generate ./env/...
//
//go:generate go run ../internal/gen/envgen -out zz_generated.go -package env -crd "group=gateway.networking.k8s.io;packages=sigs.k8s.io/gateway-api/apis/v1;cluster-scoped=GatewayClass" -crd "group=monitoring.coreos.com;packages=github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1;assert-name-overrides=Probe:MonitoringProbe"
package env
