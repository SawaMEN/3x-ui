package service

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// Node routing owns this namespace, including identities provisioned by older
// panels. Classification does not depend on mutable group/comment fields.
const nodeRouteClientPattern = "node-route-%@internal"

var ErrManagedNodeRouteClient = errors.New("подключение ноды управляется в разделе маршрутизации")

func IsManagedNodeRouteClientEmail(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	return strings.HasPrefix(email, nodeRoutePrefix) && strings.HasSuffix(email, "@internal")
}

func publicClientRecords(tx *gorm.DB, emailColumn string) *gorm.DB {
	return tx.Where("LOWER(TRIM("+emailColumn+")) NOT LIKE ?", nodeRouteClientPattern)
}

func rejectManagedNodeRouteEmails(emails []string) error {
	for _, email := range emails {
		if IsManagedNodeRouteClientEmail(email) {
			return ErrManagedNodeRouteClient
		}
	}
	return nil
}
