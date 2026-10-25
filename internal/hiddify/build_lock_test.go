package hiddify

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestRuntimeIdentityMatchesBuildLock(t *testing.T) {
	data, err := os.ReadFile("../../docs/hiddify-core-target.json")
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		Target struct {
			Version    string            `json:"version"`
			Commit     string            `json:"commit"`
			Submodules map[string]string `json:"submodules"`
		} `json:"target"`
	}
	if err = json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	if TargetVersion != lock.Target.Version || TargetRevision != lock.Target.Commit {
		t.Fatal("runtime identity differs from build lock")
	}
	coreRevision := lock.Target.Submodules["hiddify-sing-box"]
	if len(coreRevision) < 12 || !strings.Contains(TargetVersion, coreRevision[:12]) {
		t.Fatal("runtime version must identify pinned sing-box")
	}
}
