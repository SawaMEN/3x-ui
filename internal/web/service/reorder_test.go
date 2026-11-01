package service

import "testing"

func TestValidateReorderIDs(t *testing.T) {
	valid := []int{3, 1, 2}
	if err := validateReorderIDs(valid); err != nil {
		t.Fatalf("valid ids rejected: %v", err)
	}

	for name, ids := range map[string][]int{
		"empty":      nil,
		"zero":       {1, 0, 2},
		"negative":   {1, -2},
		"duplicates": {1, 2, 1},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateReorderIDs(ids); err == nil {
				t.Fatalf("expected %s payload to be rejected", name)
			}
		})
	}
}
