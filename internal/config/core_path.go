package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// OwnerCoreBinaryPath is shared with sidecars without importing core adapters.
func OwnerCoreBinaryPath() string {
	name := fmt.Sprintf("hiddify-core-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(GetBinFolderPath(), name)
	if filepath.IsAbs(path) {
		return path
	}
	executable, err := os.Executable()
	if err != nil {
		executable = "."
	}
	return filepath.Join(filepath.Dir(executable), path)
}

// StandaloneCoreBinaryPath keeps separately installed engines beside panel assets.
func StandaloneCoreBinaryPath(engine string) string {
	var name string
	switch engine {
	case "xray":
		name = fmt.Sprintf("xray-%s-%s", runtime.GOOS, runtime.GOARCH)
	case "sing-box-upstream":
		name = fmt.Sprintf("sing-box-upstream-%s-%s", runtime.GOOS, runtime.GOARCH)
	default:
		return ""
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(filepath.Dir(OwnerCoreBinaryPath()), name)
}
