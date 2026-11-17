package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

func telemtWebRenewalMode(data []byte) string {
	auth, ownRoot := "", false
	section := "renewalparams"
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.Trim(line, "[] ")
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "\"', ")
		if key == "authenticator" && section == "renewalparams" {
			auth = value
		}
		if value == telemtWebDecoyDir && ((section == "renewalparams" && key == "webroot_path") || section == "webroot_map") {
			ownRoot = true
		}
	}
	if auth == "webroot" && ownRoot {
		return "managed-webroot"
	}
	if auth == "standalone" {
		return "standalone"
	}
	return "external"
}

func telemtWebRenewalArguments(data []byte, frontend, domain string) []string {
	mode := telemtWebRenewalMode(data)
	if frontend == "nginx" && mode == "standalone" {
		return []string{"reconfigure", "--cert-name", domain, "--non-interactive", "--webroot", "--webroot-path", telemtWebDecoyDir}
	}
	if frontend != "nginx" && mode == "managed-webroot" {
		return []string{"reconfigure", "--cert-name", domain, "--non-interactive", "--standalone"}
	}
	return nil
}

func telemtWebPrepareCertificateRenewal(ctx context.Context, state TelemtWebProxyState, domain, cert string) error {
	if cert != "/etc/letsencrypt/live/"+domain+"/fullchain.pem" {
		return nil
	}
	data, err := os.ReadFile("/etc/letsencrypt/renewal/" + domain + ".conf")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	args := telemtWebRenewalArguments(data, telemtWebFrontend(state), domain)
	if len(args) == 0 {
		return nil
	}
	if telemtWebFrontend(state) == "nginx" {
		if err := telemtWebWriteAcmeNginxConfig(domain); err != nil {
			return err
		}
		if err := telemtWebEnsureNginxRunning(); err != nil {
			return err
		}
		if err := telemtWebReloadNginx(); err != nil {
			return err
		}
	} else if !telemtWebPortAvailable(80) {
		return errors.New("switching this certificate to standalone renewal requires free port 80")
	}
	if err := runTelemtWebCommand(ctx, append([]string{"certbot"}, args...)...); err != nil {
		return fmt.Errorf("certificate renewal reconfiguration failed (requires Certbot 2.3+): %w", err)
	}
	return nil
}
