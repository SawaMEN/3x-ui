package service

import (
    "reflect"
    "testing"
)

func TestNodeRouteTagRoundTrip(t *testing.T) {
    for _, id := range []int{1, 42, 123456} {
        tag := nodeRouteTag(id)
        got, ok := nodeRouteInboundID(tag)
        if !ok || got != id {
            t.Fatalf("invalid roundtrip for %q: %d %v", tag, got, ok)
        }
    }
    for _, value := range []string{
        "", "node-route-", "node-route-0", "node-route-01",
        "node-route--1", "node-route-1a", "node-route- 2",
        "node-route-9999999999999999999999999999", "direct", "node-egress-3",
    } {
        if _, ok := nodeRouteInboundID(value); ok {
            t.Errorf("accepted unsafe node route tag: %q", value)
        }
    }
}

func TestNodeRouteReferencedDestinationsOnly(t *testing.T) {
    rules := []map[string]any{
        {"outbound": "direct"},
        {"outbound": "node-route-3"},
        {"outbound": "node-route-3"},
        {"outbound": "node-route-4", "enabled": false},
        {"outbound": "node-route-9", "action": "reject"},
        {"outbound": "node-route-7"},
    }
    if got := nodeRouteIDs(rules, "outbound"); !reflect.DeepEqual(got, []int{3, 7}) {
        t.Fatalf("unexpected node routes: %v", got)
    }
    rules = []map[string]any{
        {"outboundTag": "node-route-2"},
        {"balancerTag": "node-route-5"},
    }
    if got := nodeRouteIDs(rules, "outboundTag"); !reflect.DeepEqual(got, []int{2}) {
        t.Fatalf("xray should ignore balancer tags: %v", got)
    }
}

func TestNodeRouteProtocolCompatibility(t *testing.T) {
    for _, protocol := range []string{"vless", "vmess", "trojan", "shadowsocks"} {
        if !nodeRouteProtocolSupported(protocol, "xray") || !nodeRouteProtocolSupported(protocol, "sing-box") {
            t.Errorf("%q must be supported by both routing engines", protocol)
        }
    }
    for _, protocol := range []string{"anytls", "tuic", "hysteria", "naive"} {
        if nodeRouteProtocolSupported(protocol, "xray") || !nodeRouteProtocolSupported(protocol, "sing-box") {
            t.Errorf("%q should be native-sing-box only", protocol)
        }
    }
    if nodeRouteProtocolSupported("unknown", "sing-box") {
        t.Fatal("unknown protocol must be disabled")
    }
}
