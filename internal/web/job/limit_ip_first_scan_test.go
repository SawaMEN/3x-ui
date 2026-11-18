package job

import (
	"testing"
	"time"

)

func TestProcessObservedAppliesIPLimitOnFirstScan(t *testing.T) {
	setupIntegrationDB(t)
	const email = "first-scan-limit@example.test"
	seedLinkedInboundWithClient(t, "native-first-scan", email, 1)

	now := time.Now().Unix()
	observed := map[string]map[string]int64{
		email: {
			"198.51.100.1": now - 2,
			"198.51.100.2": now - 1,
		},
	}
	job := NewCheckClientIpJob()
	job.processObserved(observed, true, true)

	kept := readClientIps(t, email)
	if len(kept) != 1 || kept[0].IP != "198.51.100.2" {
		t.Fatalf("first scan must enforce limit and keep newest IP: %+v", kept)
	}
	if len(job.disAllowedIps) != 1 || job.disAllowedIps[0] != "198.51.100.1" {
		t.Fatalf("oldest IP must be selected for disconnection: %v", job.disAllowedIps)
	}
}
