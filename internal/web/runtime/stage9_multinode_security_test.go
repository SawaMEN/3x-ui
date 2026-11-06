package runtime

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestStage9RemoteCacheAndIdentityIsolatedPerNode(t *testing.T) {
	manager := NewManager(LocalDeps{})
	leftNode := &model.Node{Id: 911, Name: "left", Scheme: "https", Address: "left.example.net", Port: 443, ApiToken: "left-token", Enable: true}
	rightNode := &model.Node{Id: 912, Name: "right", Scheme: "https", Address: "right.example.net", Port: 443, ApiToken: "right-token", Enable: true}
	left, err := manager.RemoteFor(leftNode)
	if err != nil {
		t.Fatal(err)
	}
	right, err := manager.RemoteFor(rightNode)
	if err != nil {
		t.Fatal(err)
	}
	if left == right {
		t.Fatal("two independent nodes shared a mutable remote instance")
	}
	left.cacheSet("shared", 101)
	right.cacheSet("shared", 202)
	for _, tc := range []struct {
		remote *Remote
		tag string
		want int
	}{
		{left, "shared", 101}, {right, "shared", 202},
		{left, "n911-shared", 101}, {right, "n912-shared", 202},
	} {
		got, ok := tc.remote.cacheGetTag(tc.tag)
		if !ok || got != tc.want {
			t.Fatalf("node-local cache lookup %q: got=%d ok=%v want=%d", tc.tag, got, ok, tc.want)
		}
	}
	leftNode.ApiToken = "mutated-outside-runtime"
	if left.node.ApiToken != "left-token" {
		t.Fatal("runtime retained caller-owned credentials instead of cloning node state")
	}
	manager.InvalidateNode(leftNode.Id)
	replacement, err := manager.RemoteFor(leftNode)
	if err != nil {
		t.Fatal(err)
	}
	if replacement == left {
		t.Fatal("invalidation retained old remote credentials/cache")
	}
	if _, ok := replacement.cacheGetTag("shared"); ok {
		t.Fatal("new remote inherited cached inbound ids from previous node generation")
	}
	again, err := manager.RemoteFor(rightNode)
	if err != nil {
		t.Fatal(err)
	}
	if again != right {
		t.Fatal("invalidating one node discarded another node's remote")
	}
}
