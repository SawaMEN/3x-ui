package externalvpn

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Build the client container from an official pinned CLI package. This neither
// installs a Debian package on the host nor executes its systemd scripts.
func InstallFPTNClient(ctx context.Context) error {
	externalVPNUpdateMu.Lock()
	defer externalVPNUpdateMu.Unlock()
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return fmt.Errorf("FPTN client requires Linux amd64 or arm64")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("install Docker before configuring FPTN: %w", err)
	}
	spec := releaseSpec{protocol: "fptn", api: "https://api.github.com/repos/fptn-project/fptn/releases/tags/0.4.6"}
	rel, err := fetchLatestRelease(ctx, spec)
	if err != nil {
		return err
	}
	var asset releaseAsset
	for _, a := range rel.Assets {
		if a.Name == "fptn-client-cli-0.4.6-ubuntu22.04-"+runtime.GOARCH+".deb" {
			asset = a
			break
		}
	}
	if asset.URL == "" || !strings.HasPrefix(asset.Digest, "sha256:") {
		return fmt.Errorf("no verified FPTN CLI package for this architecture")
	}
	folder, err := os.MkdirTemp("", "3x-ui-fptn-client-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(folder)
	path := filepath.Join(folder, "client.deb")
	if err := downloadReleaseAsset(ctx, asset.URL, path); err != nil {
		return err
	}
	if err := verifyReleaseDigest(path, asset.Digest); err != nil {
		return err
	}
	dockerfile := `FROM ubuntu:22.04
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends iptables iproute2 net-tools ca-certificates libstdc++6 && rm -rf /var/lib/apt/lists/*
COPY client.deb /tmp/client.deb
RUN mkdir -p /opt/fptn-package && dpkg-deb -x /tmp/client.deb /opt/fptn-package && install -m 0755 /opt/fptn-package/usr/bin/fptn-client-cli /usr/bin/fptn-client-cli && rm -rf /opt/fptn-package /tmp/client.deb
RUN /usr/bin/fptn-client-cli --help >/dev/null
`
	if err := os.WriteFile(filepath.Join(folder, "Dockerfile"), []byte(dockerfile), 0o600); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "docker", "build", "--tag", "3x-ui-fptn-client:0.4.6", folder)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build FPTN client container: %w", err)
	}
	return nil
}
