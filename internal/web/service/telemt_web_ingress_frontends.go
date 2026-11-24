package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	telemtIngressRouterUnit = "3x-ui-telemt-ingress.service"
	telemtIngressRouterConfig = telemtWebFrontDir + "/ingress.cfg"
	telemtIngressWebSocket = telemtWebFrontDir + "/https.sock"
)

func telemtIngressUsesRouter(state TelemtWebProxyState) bool {
	ids, err := telemtIngressInboundIDs(state.SharedIngress)
	return err == nil && len(ids) > 0 && telemtWebFrontend(state) != "haproxy"
}

func telemtIngressPublicOwner(state TelemtWebProxyState) string {
	if telemtIngressUsesRouter(state) {
		return telemtIngressRouterEngine(state)
	}
	return telemtWebFrontend(state)
}

func telemtIngressEntryActive(state TelemtWebProxyState) bool {
	return telemtWebFrontendActive(state) &&
		(!telemtIngressUsesRouter(state) || telemtWebUnitExecutableActive(telemtIngressRouterUnit, telemtIngressRouterEngine(state)))
}

func telemtIngressFrontendPath(state TelemtWebProxyState) string {
	if telemtWebFrontend(state) == "nginx" {
		return telemtWebNginxConf
	}
	return telemtWebFrontDir + "/config"
}

// The optional layer-4 service forwards TLS without termination. The selected
// HTTP frontend receives the original client address via a private Unix socket.
func renderTelemtIngressRouter(state TelemtWebProxyState, plan telemtIngressPlan) []byte {
	bind := ":443"
	if telemtWebNeedsIPv6(state) {
		bind = "[::]:443 v4v6"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "global\n    maxconn 4096\n\ndefaults\n    mode tcp\n    timeout connect 5s\n    timeout client 1h\n    timeout server 1h\n\nfrontend entry\n    bind %s\n    tcp-request inspect-delay 5s\n    tcp-request content accept if { req.ssl_hello_type 1 }\n", bind)
	for _, route := range plan.proxies {
		fmt.Fprintf(&b, "    use_backend proxy_%d if { req.ssl_sni -i %s }\n", route.id, route.host)
	}
	b.WriteString("    default_backend web_tls\n")
	for _, route := range plan.proxies {
		fmt.Fprintf(&b, "\nbackend proxy_%d\n    server local %s\n", route.id, route.address)
	}
	fmt.Fprintf(&b, "\nbackend web_tls\n    server web %s send-proxy-v2\n", telemtIngressWebSocket)
	return []byte(b.String())
}

func renderTelemtIngressNginx(state TelemtWebProxyState, plan telemtIngressPlan) []byte {
	var b strings.Builder
	b.WriteString("map $http_upgrade $xui_telemt_connection_upgrade { default upgrade; '' ''; }\n")
	b.WriteString("map $sent_http_content_type $xui_telemt_cover_csp { default ''; ~*^text/html \"sandbox allow-scripts; connect-src 'none'; form-action 'none'; base-uri 'none'; frame-ancestors 'none'\"; }\n")
	listen := "listen 443 ssl http2;"
	realIP := ""
	if telemtIngressUsesRouter(state) {
		listen = "listen unix:" + telemtIngressWebSocket + " ssl http2 proxy_protocol;"
		realIP = "    set_real_ip_from unix:;\n    real_ip_header proxy_protocol;\n"
	} else if telemtWebNeedsIPv6(state) {
		listen += "\n    listen [::]:443 ssl http2;"
	}
	proxy := func(route telemtIngressRoute, cover bool) string {
		scheme := "http"
		if route.certFile != "" {
			scheme = "https"
		}
		var p strings.Builder
		fmt.Fprintf(&p, "        proxy_pass %s://%s;\n", scheme, route.address)
		p.WriteString("        proxy_http_version 1.1;\n        proxy_set_header Host $host;\n        proxy_set_header Forwarded '';\n        proxy_set_header X-Real-IP $remote_addr;\n        proxy_set_header X-Forwarded-For $remote_addr;\n        proxy_set_header X-Forwarded-Proto https;\n        proxy_set_header X-Forwarded-Host $host;\n        proxy_set_header X-Forwarded-Port 443;\n        proxy_set_header Upgrade $http_upgrade;\n        proxy_set_header Connection $xui_telemt_connection_upgrade;\n        proxy_connect_timeout 5s;\n        proxy_read_timeout 65s;\n        proxy_send_timeout 65s;\n        proxy_request_buffering off;\n        proxy_buffering off;\n        proxy_next_upstream off;\n")
		if route.certFile != "" {
			fmt.Fprintf(&p, "        proxy_ssl_verify on;\n        proxy_ssl_trusted_certificate %s;\n        proxy_ssl_server_name on;\n        proxy_ssl_name %s;\n", strconv.Quote(route.certFile), route.tlsName)
		}
		if cover {
			p.WriteString("        add_header Content-Security-Policy $xui_telemt_cover_csp always;\n")
		}
		return p.String()
	}
	for _, host := range telemtWebHosts(state) {
		fmt.Fprintf(&b, "\nserver {\n    %s\n    server_name %s;\n    ssl_certificate %s;\n    ssl_certificate_key %s;\n    ssl_protocols TLSv1.2 TLSv1.3;\n    access_log off;\n    client_max_body_size 2m;\n%s    if ($host != %s) { return 404; }\n    if ($ssl_server_name != $host) { return 421; }\n    if ($request_method = CONNECT) { return 405; }\n", listen, host, strconv.Quote(state.CertFile), strconv.Quote(state.KeyFile), realIP, host)
		paths := map[string]telemtIngressRoute{}
		for _, path := range plan.legacyPaths {
			paths[path] = plan.panel
		}
		for _, route := range []telemtIngressRoute{plan.panel, plan.subscriptions} {
			if route.host == host {
				for _, path := range route.paths {
					paths[path] = route
				}
			}
		}
		names := make([]string, 0, len(paths))
		for path := range paths {
			names = append(names, path)
		}
		slices.Sort(names)
		for _, path := range names {
			for _, location := range []string{"= "+path, "^~ "+path+"/"} {
				fmt.Fprintf(&b, "    location %s {\n%s    }\n", location, proxy(paths[path], false))
			}
		}
		fmt.Fprintf(&b, "    location / {\n%s    }\n}\n", proxy(telemtIngressRoute{address: net.JoinHostPort(telemtWebListenIP, strconv.Itoa(state.ListenPort))}, true))
	}
	return []byte(b.String())
}

func renderTelemtIngressCaddy(state TelemtWebProxyState, plan telemtIngressPlan) ([]byte, error) {
	proxy := func(route telemtIngressRoute, cover bool) map[string]any {
		transport := map[string]any{"protocol": "http", "versions": []string{"1.1"}}
		if route.certFile != "" {
			transport["tls"] = map[string]any{"server_name": route.tlsName, "root_ca_pem_files": []string{route.certFile}}
		}
		p := map[string]any{"handler": "reverse_proxy", "upstreams": []any{map[string]any{"dial": route.address}}, "flush_interval": -1, "transport": transport,
			"headers": map[string]any{"request": map[string]any{"delete": []string{"Forwarded"}, "set": map[string][]string{"X-Real-IP": {"{http.request.remote.host}"}, "X-Forwarded-Port": {"443"}, "X-Forwarded-For": {"{http.request.remote.host}"}, "X-Forwarded-Proto": {"https"}, "X-Forwarded-Host": {"{http.request.host}"}, "Host": {"{http.request.host}"}}}}}
		if cover && telemtWebCaddySupportsStreamDelay() {
			p["stream_close_delay"] = "5m"
		}
		return p
	}
	var routes []any
	routes = append(routes, map[string]any{"match": []any{map[string]any{"method": []string{"CONNECT"}}}, "handle": []any{map[string]any{"handler":"static_response", "status_code":405}}, "terminal": true})
	add := func(hosts, paths []string, route telemtIngressRoute) {
		if len(paths) == 0 {
			return
		}
		var patterns []string
		for _, p := range paths {
			patterns = append(patterns, p, p+"/*")
		}
		routes = append(routes, map[string]any{"match": []any{map[string]any{"host": hosts, "path": patterns}}, "handle": []any{proxy(route, false)}, "terminal": true})
	}
	add(telemtWebHosts(state), plan.legacyPaths, plan.panel)
	for _, route := range []telemtIngressRoute{plan.panel, plan.subscriptions} {
		add([]string{route.host}, route.paths, route)
	}
	coverHeaders := map[string]any{
		"handler": "headers",
		"response": map[string]any{
			"deferred": true,
			"require": map[string]any{"headers": map[string][]string{"Content-Type": {"text/html*"}}},
			"add": map[string][]string{"Content-Security-Policy": {"sandbox allow-scripts; connect-src 'none'; form-action 'none'; base-uri 'none'; frame-ancestors 'none'"}},
		},
	}
	routes = append(routes,
		map[string]any{"match": []any{map[string]any{"host":telemtWebHosts(state)}}, "handle": []any{coverHeaders, proxy(telemtIngressRoute{address:net.JoinHostPort(telemtWebListenIP,strconv.Itoa(state.ListenPort))},true)}, "terminal":true},
		map[string]any{"handle": []any{map[string]any{"handler":"static_response","status_code":404}}})
	server := map[string]any{"listen":[]string{":443"}, "protocols":[]string{"h1","h2"}, "strict_sni_host":true, "automatic_https":map[string]any{"disable":true}, "tls_connection_policies":[]any{map[string]any{}}, "routes":routes}
	if telemtIngressUsesRouter(state) {
		server["listen"] = []string{"unix/"+telemtIngressWebSocket}
		server["listener_wrappers"] = []any{map[string]any{"wrapper":"proxy_protocol","timeout":"5s"}, map[string]any{"wrapper":"tls"}}
	}
	config := map[string]any{"admin": map[string]any{"listen":"unix//run/3x-ui-telemt-front/caddy.sock","config":map[string]any{"persist":false}}, "apps":map[string]any{"tls":map[string]any{"certificates":map[string]any{"load_files":[]any{map[string]any{"certificate":state.CertFile,"key":state.KeyFile}}}}, "http":map[string]any{"servers":map[string]any{"web":server}}}}
	return json.MarshalIndent(config, "", "  ")
}

func renderTelemtIngressFrontend(state TelemtWebProxyState, plan telemtIngressPlan) ([]byte,error) {
	switch telemtWebFrontend(state) {
	case "nginx":
		return renderTelemtIngressNginx(state,plan),nil
	case "caddy":
		return renderTelemtIngressCaddy(state,plan)
	default:
		return renderTelemtWebIngress(state,plan),nil
	}
}

func writeTelemtIngressFrontend(state TelemtWebProxyState, plan telemtIngressPlan) error {
	data,err := renderTelemtIngressFrontend(state,plan)
	if err != nil { return err }
	if telemtIngressUsesRouter(state) {
		if err := os.MkdirAll(telemtWebFrontDir,0700); err != nil { return err }
		if err := os.Chmod(telemtWebFrontDir,0700); err != nil { return err }
		routerData, err := telemtIngressRouterData(state, plan)
		if err != nil {
			return err
		}
		if err := telemtIngressWriteRouterConfig(state, routerData); err != nil {
			return err
		}
	}
	if telemtWebFrontend(state) == "nginx" {
		if err := os.MkdirAll(filepath.Dir(telemtWebNginxConf),0755); err != nil { return err }
		return writeTelemtWebFrontendFile(telemtWebNginxConf,data)
	}
	return writeTelemtWebFrontendFiles(state,data)
}

func telemtIngressEnsureRouter(state TelemtWebProxyState) error {
	if !state.Enabled || !telemtIngressUsesRouter(state) {
		return nil
	}
	engine := telemtIngressRouterEngine(state)
	binary, err := exec.LookPath(engine)
	if err != nil { return err }
	if err := telemtIngressValidateRouterConfig(state, telemtIngressRouterConfig); err != nil {
		return err
	}
	dependency := telemtWebFrontUnit
	if telemtWebFrontend(state) == "nginx" { dependency = "nginx.service" }
	args := telemtIngressRouterStartArgs(engine, telemtIngressRouterConfig)
	quoted := make([]string, 0, len(args)+1)
	quoted = append(quoted, strconv.Quote(binary))
	for _, arg := range args {
		quoted = append(quoted, strconv.Quote(arg))
	}
	unit := fmt.Sprintf("[Unit]\nDescription=3X-UI shared TLS/SNI router (%s)\nAfter=network-online.target %s\nRequires=%s\n\n[Service]\nType=simple\nExecStart=%s\nExecReload=%s\nRestart=on-failure\nRestartSec=2\nLimitNOFILE=65536\nUMask=0077\nNoNewPrivileges=true\n\n[Install]\nWantedBy=multi-user.target\n", engine, dependency, dependency, strings.Join(quoted, " "), telemtIngressRouterReloadCommand(engine, binary, telemtIngressRouterConfig))
	path := filepath.Join("/etc/systemd/system",telemtIngressRouterUnit)
	old,err := os.ReadFile(path)
	if err != nil && !errors.Is(err,os.ErrNotExist) { return err }
	active := systemctl("is-active", "--quiet", telemtIngressRouterUnit) == nil
	if !bytes.Equal(old,[]byte(unit)) {
		if err := os.WriteFile(path,[]byte(unit),0644); err != nil { return err }
		if err := systemctl("daemon-reload"); err != nil { return err }
		if active { return systemctl("restart",telemtIngressRouterUnit) }
	}
	if active {
		if !telemtWebUnitExecutableActive(telemtIngressRouterUnit, engine) {
			return systemctl("restart", telemtIngressRouterUnit)
		}
		return systemctl("reload", telemtIngressRouterUnit)
	}
	return systemctl("enable","--now",telemtIngressRouterUnit)
}

func telemtIngressRemoveRouter(state TelemtWebProxyState) error {
	if !telemtIngressUsesRouter(state) { return nil }
	unit := filepath.Join("/etc/systemd/system",telemtIngressRouterUnit)
	if _,err := os.Stat(unit); err == nil {
		if err := systemctl("disable","--now",telemtIngressRouterUnit); err != nil { return err }
		if err := os.Remove(unit); err != nil { return err }
		if err := systemctl("daemon-reload"); err != nil { return err }
	} else if !errors.Is(err,os.ErrNotExist) { return err }
	if err := os.Remove(telemtIngressRouterConfig); err != nil && !errors.Is(err,os.ErrNotExist) { return err }
	return nil
}

// Do not move an unrelated Nginx HTTPS site off 443. Inspect its loaded
// configuration before changing the managed virtual host.
func telemtIngressNginxHasForeign443(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "nginx", "-T").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("nginx -T: %w", err)
	}
	return nginxHasForeignTCP443(string(out), telemtWebNginxConf, telemtWebAcmeConf), nil
}

func telemtIngressValidateRouterConfig(state TelemtWebProxyState, path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	engine := telemtIngressRouterEngine(state)
	out, err := exec.CommandContext(ctx, engine, telemtIngressRouterValidateArgs(engine, path)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("shared TCP router validation failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Validate a candidate before replacing the currently usable router config.
func telemtIngressWriteRouterConfig(state TelemtWebProxyState, data []byte) error {
	tmp, err := os.CreateTemp(telemtWebFrontDir, ".ingress-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, writeErr := tmp.Write(data)
	if err := errors.Join(writeErr, tmp.Close()); err != nil {
		return err
	}
	if err := telemtIngressValidateRouterConfig(state, tmp.Name()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), telemtIngressRouterConfig)
}

func telemtIngressCaddyProxyProtocolAvailable(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "caddy", "list-modules").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == "caddy.listeners.proxy_protocol" {
			return true
		}
	}
	return false
}
