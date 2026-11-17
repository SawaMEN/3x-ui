package service

import (
	"net"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
)

// Certificate ownership alone does not establish an HTTPS handshake target.
// Wildcard certificates also need a concrete hostname before use as SNI.
func managedSNIRealityTarget(name string, port int) string {
	if port < 1 || port > 65535 || strings.HasPrefix(name, "*.") {
		return ""
	}
	if host, err := netsafe.NormalizeHost(name); err != nil || net.ParseIP(host) != nil {
		return ""
	}
	return net.JoinHostPort(name, strconv.Itoa(port))
}

func normalizeManagedSNIRealityTarget(name, target string) string {
	host, rawPort, err := net.SplitHostPort(target)
	if err != nil || !strings.EqualFold(strings.TrimSuffix(host, "."), name) {
		return ""
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		return ""
	}
	return managedSNIRealityTarget(name, port)
}
