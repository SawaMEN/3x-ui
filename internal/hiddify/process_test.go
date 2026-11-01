package hiddify

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestPinnedProfileRequiresEveryBuildTag(t *testing.T) {
	for _, missing := range requiredBuildTags {
		t.Run(missing, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XUI_BIN_FOLDER", dir)
			var tags []string
			for _, tag := range requiredBuildTags {
				if tag != missing {
					tags = append(tags, tag)
				}
			}
			script := fmt.Sprintf("#!/bin/sh\nprintf 'hiddify-core version %s\\nTags: %s\\nRevision: %s\\n'\n", TargetVersion, strings.Join(tags, ","), TargetRevision)
			if err := os.WriteFile(GetBinaryPath(), []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := NewProcess(GetConfigPath(), true).Version(context.Background()); err == nil || !strings.Contains(err.Error(), missing) {
				t.Fatalf("missing %s accepted: %v", missing, err)
			}
		})
	}
}
