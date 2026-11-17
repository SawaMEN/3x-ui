package sidecartraffic

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// RevisionOf identifies the saved runtime inputs, excluding counters and panel
// presentation fields. Local filtering carries the original revision so removing
// depleted clients does not look like a different saved configuration.
func RevisionOf(inbound *model.Inbound) string {
	if inbound.RuntimeSourceRevision != "" {
		return inbound.RuntimeSourceRevision
	}
	data, _ := json.Marshal(struct {
		ID          int
		Protocol    model.Protocol
		Tag, Listen string
		Port        int
		Settings    string
	}{inbound.Id, inbound.Protocol, inbound.Tag, inbound.Listen, inbound.Port, inbound.Settings})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// DisabledRevision fingerprints eligibility in both directions, so a delayed
// snapshot cannot undo either a quota block or a subsequent quota reset.
func DisabledRevision[T any](disabled map[string]T) string {
	if len(disabled) == 0 {
		return ""
	}
	emails := make([]string, 0, len(disabled))
	for email := range disabled {
		emails = append(emails, email)
	}
	slices.Sort(emails)
	data, _ := json.Marshal(emails)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
