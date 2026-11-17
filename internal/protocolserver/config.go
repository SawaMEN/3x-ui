package protocolserver

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/naiveproxy"
)

const InternalUserPrefix = "__panel_hysteria_"

type Plan struct {
	Tag         string
	Protocol    model.Protocol
	Binary      string
	Config      json.RawMessage
	Files       map[string]string
	AuthListen  string
	AuthPath    string
	Users       map[string]string
	StatsListen string
	StatsSecret string
	ReadyListen string
	ReadyUDP    bool
}

func Supported(protocol model.Protocol, goos, goarch string) bool {
	if goos != "linux" {
		return false
	}
	if protocol == model.NaiveProxy {
		return goarch == "amd64"
	}
	return protocol == model.Hysteria && (goarch == "amd64" || goarch == "arm64")
}

func BinaryPath(protocol model.Protocol) string {
	if protocol == model.NaiveProxy {
		return naiveproxy.BinaryPath()
	}
	return filepath.Join(config.GetBinFolderPath(), "hysteria")
}

func Installed(protocol model.Protocol) bool {
	if !Supported(protocol, runtime.GOOS, runtime.GOARCH) {
		return false
	}
	info, err := os.Stat(BinaryPath(protocol))
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func text(raw map[string]any, key string) string { value, _ := raw[key].(string); return value }
func object(raw map[string]any, key string) map[string]any {
	value, _ := raw[key].(map[string]any)
	return value
}

func integer(raw map[string]any, key string) int {
	value, _ := strconv.Atoi(fmt.Sprint(raw[key]))
	return value
}

func objects(value any) []map[string]any {
	data, _ := json.Marshal(value)
	var out []map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func secret(raw map[string]any, purpose string) string {
	data, _ := json.Marshal(raw)
	sum := sha256.Sum256(append([]byte(purpose+":"), data...))
	return hex.EncodeToString(sum[:])
}

// Build replaces the public native listener with an authenticated loopback bridge.
// Both the bridge and the external server retain the original panel identity.
func Build(raw map[string]any, ports [3]int) (Plan, map[string]any, error) {
	protocol := model.NaiveProxy
	if text(raw, "type") == "hysteria2" {
		protocol = model.Hysteria
	}
	plan := Plan{Tag: text(raw, "tag"), Protocol: protocol, Binary: BinaryPath(protocol)}
	listen := text(raw, "listen")
	if listen == "" {
		listen = "0.0.0.0"
	}
	address := net.JoinHostPort(strings.Trim(listen, "[]"), strconv.Itoa(integer(raw, "listen_port")))
	readyHost := strings.Trim(listen, "[]")
	switch readyHost {
	case "0.0.0.0":
		readyHost = "127.0.0.1"
	case "::":
		readyHost = "::1"
	}
	plan.ReadyListen = net.JoinHostPort(readyHost, strconv.Itoa(integer(raw, "listen_port")))
	bridgeAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[0]))
	bridge := map[string]any{"type": "socks", "tag": plan.Tag, "listen": "127.0.0.1", "listen_port": ports[0]}
	users := objects(raw["users"])
	if len(users) == 0 {
		return Plan{}, nil, fmt.Errorf("%s has no active users", plan.Tag)
	}
	tls := object(raw, "tls")
	cert, key := text(tls, "certificate_path"), text(tls, "key_path")
	var pem struct {
		Certificate []string `json:"certificate"`
		Key         []string `json:"key"`
	}
	pemData, _ := json.Marshal(tls)
	_ = json.Unmarshal(pemData, &pem)
	certificatePEM, keyPEM := strings.Join(pem.Certificate, "\n"), strings.Join(pem.Key, "\n")
	inline := cert == "" && key == "" && certificatePEM != "" && keyPEM != ""
	if !inline && (cert == "" || key == "") {
		return Plan{}, nil, fmt.Errorf("%s requires a TLS certificate and private key", plan.Tag)
	}
	inlineDir := filepath.Join(config.GetBinFolderPath(), "protocol-servers", identifier(plan.Tag))
	var config map[string]any
	if protocol == model.NaiveProxy {
		var routes, bridgeUsers []map[string]any
		for _, user := range users {
			name, password := text(user, "username"), text(user, "password")
			if name == "" || password == "" || len(name) > 255 || len(password) > 255 || strings.Contains(name, ":") {
				return Plan{}, nil, fmt.Errorf("%s has invalid Naive credentials", plan.Tag)
			}
			auth := base64.StdEncoding.EncodeToString([]byte(name + ":" + password))
			upstream := url.URL{Scheme: "socks5", Host: bridgeAddr, User: url.UserPassword(name, password)}
			handler := map[string]any{"handler": "forward_proxy", "auth_credentials": [][]byte{[]byte(auth)}, "hide_ip": true, "hide_via": true, "probe_resistance": map[string]any{}, "upstream": upstream.String()}
			routes = append(routes, map[string]any{"match": []any{map[string]any{"header": map[string]any{"Proxy-Authorization": []string{"Basic " + auth}}}}, "handle": []any{handler}})
			bridgeUsers = append(bridgeUsers, map[string]any{"username": name, "password": password})
		}
		routes = append(routes, map[string]any{"handle": []any{map[string]any{"handler": "static_response", "status_code": 404}}})
		bridge["users"] = bridgeUsers
		protocols := []string{"h1", "h2", "h3"}
		if text(raw, "network") == "tcp" {
			protocols = []string{"h1", "h2"}
		} else if text(raw, "network") == "udp" {
			protocols = []string{"h3"}
			plan.ReadyUDP = true
		}
		server := map[string]any{"listen": []string{address}, "routes": routes, "protocols": protocols, "tls_connection_policies": []any{map[string]any{}}, "automatic_https": map[string]any{"disable": true}}
		certificates := map[string]any{"load_files": []any{map[string]any{"certificate": cert, "key": key}}}
		if inline {
			certificates = map[string]any{"load_pem": []any{map[string]any{"certificate": certificatePEM, "key": keyPEM}}}
		}
		config = map[string]any{"admin": map[string]any{"disabled": true}, "apps": map[string]any{"http": map[string]any{"servers": map[string]any{"proxy": server}}, "tls": map[string]any{"certificates": certificates}}}
	} else {
		if inline {
			dir := inlineDir
			cert, key = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
			plan.Files = map[string]string{"cert.pem": certificatePEM, "key.pem": keyPEM}
		}
		bridgeUser := InternalUserPrefix + secret(raw, "user")[:16]
		password := secret(raw, "bridge")
		bridge["users"] = []map[string]any{{"username": bridgeUser, "password": password}}
		plan.AuthListen = net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[1]))
		plan.AuthPath = "/" + secret(raw, "auth")
		plan.StatsListen = net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[2]))
		plan.StatsSecret = secret(raw, "stats")
		plan.Users = map[string]string{}
		for _, user := range users {
			name, password := text(user, "name"), text(user, "password")
			if name == "" || password == "" {
				return Plan{}, nil, fmt.Errorf("%s has invalid Hysteria credentials", plan.Tag)
			}
			if other, exists := plan.Users[password]; exists && other != name {
				return Plan{}, nil, fmt.Errorf("%s has duplicate Hysteria authentication tokens", plan.Tag)
			}
			plan.Users[password] = name
		}
		config = map[string]any{"listen": address, "tls": map[string]any{"cert": cert, "key": key}, "auth": map[string]any{"type": "http", "http": map[string]any{"url": "http://" + plan.AuthListen + plan.AuthPath}}, "trafficStats": map[string]any{"listen": plan.StatsListen, "secret": plan.StatsSecret}, "outbounds": []any{map[string]any{"name": "panel", "type": "socks5", "socks5": map[string]any{"addr": bridgeAddr, "username": bridgeUser, "password": password}}}}
		if ignore, ok := raw["ignore_client_bandwidth"].(bool); ok {
			config["ignoreClientBandwidth"] = ignore
		}
		bandwidth := map[string]any{}
		if up := integer(raw, "up_mbps"); up > 0 {
			bandwidth["up"] = fmt.Sprintf("%d mbps", up)
		}
		if down := integer(raw, "down_mbps"); down > 0 {
			bandwidth["down"] = fmt.Sprintf("%d mbps", down)
		}
		if len(bandwidth) > 0 {
			config["bandwidth"] = bandwidth
		}
		if obfs := object(raw, "obfs"); len(obfs) > 0 {
			kind := text(obfs, "type")
			if kind != "salamander" {
				return Plan{}, nil, fmt.Errorf("%s: standalone Hysteria does not support %s obfuscation", plan.Tag, kind)
			}
			config["obfs"] = map[string]any{"type": kind, kind: map[string]any{"password": text(obfs, "password")}}
		}
		if masquerade := object(raw, "masquerade"); len(masquerade) > 0 {
			kind := text(masquerade, "type")
			options := map[string]any{}
			switch kind {
			case "proxy":
				options["url"] = masquerade["url"]
				options["rewriteHost"] = masquerade["rewrite_host"]
			case "file":
				options["dir"] = masquerade["directory"]
			case "string":
				options["content"] = masquerade["content"]
				options["statusCode"] = masquerade["status_code"]
				options["headers"] = masquerade["headers"]
			default:
				return Plan{}, nil, fmt.Errorf("%s: invalid masquerade type %q", plan.Tag, kind)
			}
			config["masquerade"] = map[string]any{"type": kind, kind: options}
		}
	}
	data, err := json.MarshalIndent(config, "", "  ")
	plan.Config = data
	return plan, bridge, err
}
