package service

import (
	"reflect"
	"testing"
)

func TestRunningAutomaticSelectorsUsesAppliedConfiguration(t *testing.T) {
	cfg := []byte(`{
		"outbounds": [
			{"type":"direct","tag":"b"},
			{"type":"selector","tag":"sub-auto-2"},
			{"type":"urltest","tag":"sub-auto-3"},
			{"type":"selector","tag":"sub-auto-4"}
		]
	}`)
	tags, err := runningAutomaticSelectors(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"sub-auto-2": true, "sub-auto-4": true}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("got %v, want %v", tags, want)
	}
	if _, err := runningAutomaticSelectors(nil); err == nil {
		t.Fatal("empty applied config must fail")
	}
	if _, err := runningAutomaticSelectors([]byte("invalid")); err == nil {
		t.Fatal("invalid applied config must fail")
	}
}
