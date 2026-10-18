package olcrtc

import (
	"bytes"
	"strings"
	"testing"
)

func TestArchInstallerPatchAddsPacmanWithoutRemovingApt(t *testing.T) {
	original := []byte(`#!/usr/bin/env bash
install_packages() {
	if command -v apt-get >/dev/null 2>&1; then
		apt-get update
		return
	fi
	die "unsupported OS: this installer currently supports apt-based Linux distributions"
}
`)
	patched, err := addArchPackagesToInstaller(original)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(patched, original) {
		t.Fatal("installer was not patched")
	}
	arch := bytes.Index(patched, []byte("command -v pacman"))
	apt := bytes.Index(patched, []byte("command -v apt-get"))
	if arch == -1 || apt == -1 || arch >= apt {
		t.Fatalf("Arch branch should precede apt and preserve Debian/Ubuntu: %s", patched)
	}
	if !bytes.Contains(patched, []byte("pacman -S --noconfirm --needed")) {
		t.Fatal("Arch package installation is missing")
	}
	if bytes.Contains(patched, []byte("pacman -Sy")) {
		t.Fatal("must never force a partial/full OS upgrade from the installer")
	}
	if !bytes.Contains(patched, []byte("apt-get update")) {
		t.Fatal("apt branch has been removed")
	}
}

func TestArchInstallerPatchRejectsUnexpectedScript(t *testing.T) {
	for _, input := range []string{
		"#!/usr/bin/env bash\necho nothing\n",
		"install_packages() {\ninstall_packages() {\nunsupported OS: this installer currently supports apt-based Linux distributions",
	} {
		if _, err := addArchPackagesToInstaller([]byte(input)); err == nil {
			t.Fatal("expected a changed installer to be rejected")
		}
	}
}

func TestArchInstallerPackagesContainRequiredTools(t *testing.T) {
	got, err := addArchPackagesToInstaller([]byte("install_packages() {\nunsupported OS: this installer currently supports apt-based Linux distributions"))
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range []string{"curl", "git", "tar", "xz", "iproute2", "iptables", "openssl", "ca-certificates"} {
		if !strings.Contains(string(got), pkg) {
			t.Fatalf("missing Arch dependency %s", pkg)
		}
	}
}
