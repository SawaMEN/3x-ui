package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/githubrelease"
)

const (
	vkTurnProxyReleaseTagSettingKey = "vkTurnProxyReleaseTag"
	vkTurnProxyManualStopSettingKey = "vkTurnProxyManualStop"
	releaseMetadataSuffix           = ".release"
	unknownInstalledRelease         = "Unknown"
)

func (s *SettingService) GetVKTurnProxyReleaseTag() (string, error) {
	return s.getString(vkTurnProxyReleaseTagSettingKey)
}

func (s *SettingService) SetVKTurnProxyReleaseTag(value string) error {
	return s.setString(vkTurnProxyReleaseTagSettingKey, strings.TrimSpace(value))
}

func (s *SettingService) GetVKTurnProxyManualStop() (bool, error) {
	return s.getBool(vkTurnProxyManualStopSettingKey)
}

func (s *SettingService) SetVKTurnProxyManualStop(value bool) error {
	return s.setBool(vkTurnProxyManualStopSettingKey, value)
}

func releaseMetadataPath(binaryPath string) string {
	return binaryPath + releaseMetadataSuffix
}

func readReleaseMetadata(binaryPath string) (string, error) {
	value, err := os.ReadFile(releaseMetadataPath(binaryPath))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(value)), nil
}

func writeReleaseMetadata(binaryPath string, tag string) error {
	tag = strings.TrimSpace(tag)
	path := releaseMetadataPath(binaryPath)
	if tag == "" {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".release-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.WriteString(tag + "\n")
	if err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func resolveInstalledReleaseTag(binaryPath string, getStored func() (string, error), setStored func(string) error) string {
	if tag, err := readReleaseMetadata(binaryPath); err == nil && tag != "" {
		if setStored != nil {
			stored := ""
			if getStored != nil {
				stored, _ = getStored()
			}
			if strings.TrimSpace(stored) != tag {
				_ = setStored(tag)
			}
		}
		return tag
	}
	if getStored != nil {
		if tag, err := getStored(); err == nil && strings.TrimSpace(tag) != "" {
			return strings.TrimSpace(tag)
		}
	}
	return unknownInstalledRelease
}

func getGitHubLatestReleaseVersion(repo string) (string, error) {
	releaseURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	client := &http.Client{Timeout: 30 * time.Second}
	release, err := githubrelease.Fetch(context.Background(), client, releaseURL)
	if err != nil {
		return "", err
	}

	tag := strings.TrimSpace(release.TagName)
	if tag == "" {
		return "", fmt.Errorf("GitHub latest release tag for %s is empty", repo)
	}
	return tag, nil
}

const maxReleaseMetadataBytes = 4 << 20

func decodeReleaseMetadata(body io.Reader, target any) error {
	raw, err := io.ReadAll(io.LimitReader(body, maxReleaseMetadataBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxReleaseMetadataBytes {
		return fmt.Errorf("release metadata exceeds %d bytes", maxReleaseMetadataBytes)
	}
	return json.Unmarshal(raw, target)
}
