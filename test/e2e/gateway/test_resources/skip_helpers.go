package test_resources

import "os"

// SkipUnreachablePorts reports whether cross-namespace listener checks targeting
// non-standard load-balancer ports (e.g. :5000) should be skipped. Set to "true"
// when running the test binary from a host whose egress firewall blocks outbound
// TCP to those ports (Amazon corp desktops fit this description). Leave unset in
// the cluster or on any runner with unrestricted outbound. The controller-side
// behaviour is unchanged; only the client-side assertion is skipped.
func SkipUnreachablePorts() bool {
	return os.Getenv("SKIP_UNREACHABLE_PORTS") == "true"
}
