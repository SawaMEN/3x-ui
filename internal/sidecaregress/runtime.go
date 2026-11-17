package sidecaregress

import (
	"encoding/json"
	"sync"
)

var runtimeReader struct {
	sync.RWMutex
	read func() []byte
}

// SetRuntimeReader supplies the active core configuration, or nil when stopped.
func SetRuntimeReader(read func() []byte) {
	runtimeReader.Lock()
	runtimeReader.read = read
	runtimeReader.Unlock()
}

// Ready prevents a sidecar from using another inbound's relay after an ID wraps
// or before the updated core configuration has actually been acknowledged.
// When the core is stopped, starting a sidecar is harmless: its bridge is
// unreachable and there is no direct fallback.
func Ready(id int, tag string, udp bool) bool {
	runtimeReader.RLock()
	read := runtimeReader.read
	runtimeReader.RUnlock()
	if read == nil {
		return true
	}
	data := read()
	if len(data) == 0 {
		return true
	}
	type listener struct {
		Type string `json:"type"`
		Tag  string `json:"tag"`
		Port int    `json:"listen_port"`
	}
	var config struct {
		Inbounds  []listener `json:"inbounds"`
		Endpoints []listener `json:"endpoints"`
	}

	if json.Unmarshal(data, &config) != nil {
		return false
	}
	port := Port(id)
	if udp {
		port++
	}
	for _, row := range config.Inbounds {
		if row.Tag == tag && row.Port == port && ((!udp && row.Type == "socks") || (udp && (row.Type == "direct" || row.Type == "hysteria2" || row.Type == "wireguard"))) {
			return true
		}
	}
	for _, row := range config.Endpoints {
		if row.Tag == tag && row.Port == port && ((!udp && row.Type == "socks") || (udp && (row.Type == "direct" || row.Type == "hysteria2" || row.Type == "wireguard"))) {
			return true
		}
	}
	return false
}
