package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func managerSNIChoices(e model.ManagerEntry) []string {
	choices := make([]string, 0, 1+len(e.SNIBackups))
	choices = append(choices, e.SNI)
	seen := map[string]bool{e.SNI: true}
	for _, candidate := range e.SNIBackups {
		if !seen[candidate] {
			choices = append(choices, candidate)
			seen[candidate] = true
		}
	}
	return choices
}

// managerChooseEntrySNI verifies each candidate against the full preflight
// rules before mutating the inbound draft. A failed candidate never changes
// the caller's inbound, and an existing published inbound remains untouched.
// The callback injection allows unit tests without external TLS probes.
func managerChooseEntrySNI(
	ib *model.Inbound,
	e model.ManagerEntry,
	apply func(*model.Inbound, model.ManagerEntry) error,
	verify func(*model.Inbound) error,
) (string, error) {
	var failures []error
	for _, sni := range managerSNIChoices(e) {
		candidate := *ib
		try := e
		try.SNI = sni
		if err := apply(&candidate, try); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", sni, err))
			continue
		}
		if err := verify(&candidate); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", sni, err))
			continue
		}
		*ib = candidate
		return sni, nil
	}
	return "", fmt.Errorf("no SNI candidate passed preflight: %w", errors.Join(failures...))
}

func (s *ManagerService) prepareManagerEntrySNI(
	ctx context.Context, owner int, ib *model.Inbound, e model.ManagerEntry,
) (string, error) {
	return managerChooseEntrySNI(ib, e,
		func(candidate *model.Inbound, entry model.ManagerEntry) error {
			return s.applyEntrySNI(owner, candidate, entry)
		},
		func(candidate *model.Inbound) error {
			return s.preflightTarget(ctx, candidate)
		},
	)
}
