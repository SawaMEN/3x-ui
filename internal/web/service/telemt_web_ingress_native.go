package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const telemtIngressCaddyAdmin = "unix/" + telemtWebFrontDir + "/ingress-admin.sock"

// Persist the resolved engine for the lifetime of the deployed configuration.
// An upgraded package must not silently change the owner of TCP 443.
func telemtIngressRouterEngine(state TelemtWebProxyState) string {
	if !telemtIngressUsesRouter(state) {
		return ""
	}
	switch state.IngressRouter {
	case "nginx":
		if telemtWebFrontend(state) == "nginx" {
			return "nginx"
		}
	case "caddy":
		if telemtWebFrontend(state) == "caddy" {
			return "caddy"
		}
	}
	return "haproxy"
}

func telemtIngressSelectRouter(ctx context.Context, state TelemtWebProxyState) (string, string) {
	if !telemtIngressUsesRouter(state) {
		return "", ""
	}
	switch telemtWebFrontend(state) {
	case "nginx":
		if module, ok := telemtIngressNginxStreamModule(ctx, "nginx"); ok {
			return "nginx", module
		}
	case "caddy":
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "caddy", "list-modules").Output()
		if err == nil && telemtIngressCaddyLayer4Modules(string(out)) {
			return "caddy", ""
		}
	}
	return "haproxy", ""
}

func telemtIngressCaddyLayer4Modules(output string) bool {
	modules := strings.Fields(output)
	for _, name := range []string{"layer4", "layer4.matchers.tls", "layer4.handlers.proxy", "caddy.listeners.proxy_protocol"} {
		if !slices.Contains(modules, name) {
			return false
		}
	}
	return true
}

// Distribution packages either compile stream statically or ship it as a
// dynamic module. Load only known local module files, not arbitrary nginx.conf
// includes (which may contain unrelated listeners).
func telemtIngressNginxStreamModule(ctx context.Context, binary string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "-V").CombinedOutput()
	if err != nil {
		return "", false
	}
	flags := strings.Fields(string(out))
	if !slices.Contains(flags, "--with-stream_ssl_preread_module") {
		return "", false
	}
	if slices.Contains(flags, "--with-stream") {
		return "", true
	}
	if !slices.Contains(flags, "--with-stream=dynamic") {
		return "", false
	}
	candidates := []string{"/usr/lib/nginx/modules/ngx_stream_module.so", "/usr/lib64/nginx/modules/ngx_stream_module.so", "/etc/nginx/modules/ngx_stream_module.so", "/usr/lib/nginx/ngx_stream_module.so"}
	for _, flag := range flags {
		if path, ok := strings.CutPrefix(flag, "--modules-path="); ok && filepath.IsAbs(path) {
			candidates = append([]string{filepath.Join(path, "ngx_stream_module.so")}, candidates...)
		}
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path, true
		}
	}
	return "", false
}

// Nginx stream cannot choose PROXY protocol per upstream. Send it to the
// private WEB socket and to private per-inbound sockets, where a second
// stream server consumes the header before forwarding the original bytes.
// The sockets live in the existing root-only directory.
func renderTelemtIngressNginxStream(state TelemtWebProxyState, plan telemtIngressPlan) []byte {
	var b strings.Builder
	if state.IngressRouterModule != "" {
		fmt.Fprintf(&b, "load_module %s;\n", strconv.Quote(state.IngressRouterModule))
	}
	fmt.Fprintf(&b, "error_log stderr;\npid %s/ingress.pid;\nworker_processes 1;\nevents { worker_connections 4096; }\nstream {\n    map $ssl_preread_server_name $xui_ingress_target {\n        default unix:%s;\n", telemtWebFrontDir, telemtIngressWebSocket)
	for _, route := range plan.proxies {
		fmt.Fprintf(&b, "        %s unix:%s/vpn-%d.sock;\n", route.host, telemtWebFrontDir, route.id)
	}
	b.WriteString("    }\n    server {\n        listen 0.0.0.0:443;\n")
	if telemtWebNeedsIPv6(state) {
		b.WriteString("        listen [::]:443 ipv6only=on;\n")
	}
	b.WriteString("        ssl_preread on;\n        preread_timeout 5s;\n        preread_buffer_size 32k;\n        proxy_connect_timeout 5s;\n        proxy_timeout 1h;\n        proxy_protocol on;\n        proxy_pass $xui_ingress_target;\n    }\n")
	for _, route := range plan.proxies {
		fmt.Fprintf(&b, "    server {\n        listen unix:%s/vpn-%d.sock proxy_protocol;\n        proxy_protocol_timeout 5s;\n        proxy_connect_timeout 5s;\n        proxy_timeout 1h;\n        proxy_pass %s;\n    }\n", telemtWebFrontDir, route.id, route.address)
	}
	b.WriteString("}\n")
	return []byte(b.String())
}

func renderTelemtIngressCaddyLayer4(state TelemtWebProxyState, plan telemtIngressPlan) ([]byte, error) {
	routes := make([]any, 0, len(plan.proxies)+1)
	for _, route := range plan.proxies {
		routes = append(routes, map[string]any{
			"match":  []any{map[string]any{"tls": map[string]any{"sni": []string{route.host}}}},
			"handle": []any{map[string]any{"handler": "proxy", "upstreams": []any{map[string]any{"dial": []string{route.address}}}}},
		})
	}
	routes = append(routes, map[string]any{
		"handle": []any{map[string]any{"handler": "proxy", "proxy_protocol": "v2", "upstreams": []any{map[string]any{"dial": []string{"unix/" + telemtIngressWebSocket}}}}},
	})
	listen := []string{"tcp4/:443"}
	if telemtWebNeedsIPv6(state) {
		listen = append(listen, "tcp6/[::]:443")
	}
	config := map[string]any{
		"admin": map[string]any{"listen": telemtIngressCaddyAdmin, "config": map[string]any{"persist": false}},
		"apps":  map[string]any{"layer4": map[string]any{"servers": map[string]any{"entry": map[string]any{"listen": listen, "routes": routes}}}},
	}
	return json.MarshalIndent(config, "", "  ")
}

func telemtIngressRouterData(state TelemtWebProxyState, plan telemtIngressPlan) ([]byte, error) {
	switch telemtIngressRouterEngine(state) {
	case "nginx":
		return renderTelemtIngressNginxStream(state, plan), nil
	case "caddy":
		return renderTelemtIngressCaddyLayer4(state, plan)
	default:
		return renderTelemtIngressRouter(state, plan), nil
	}
}

func telemtIngressRouterStartArgs(engine, path string) []string {
	switch engine {
	case "nginx":
		return []string{"-c", path, "-g", "daemon off; master_process off;"}
	case "caddy":
		return []string{"run", "--config", path}
	default:
		return []string{"-W", "-db", "-f", path}
	}
}

func telemtIngressRouterValidateArgs(engine, path string) []string {
	switch engine {
	case "nginx":
		return []string{"-t", "-c", path, "-g", "daemon off; master_process off;"}
	case "caddy":
		return []string{"validate", "--config", path}
	default:
		return []string{"-c", "-f", path}
	}
}

func telemtIngressRouterReloadCommand(engine, binary, path string) string {
	switch engine {
	case "nginx":
		return "/bin/kill -HUP $MAINPID"
	case "caddy":
		return strconv.Quote(binary) + " reload --force --address " + telemtIngressCaddyAdmin + " --config " + path
	default:
		return "/bin/kill -USR2 $MAINPID"
	}
}
