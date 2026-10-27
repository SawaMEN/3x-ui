package model

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidateConnectionName preserves an omitted override and normalizes an explicit
// empty value to the automatic naming mode. This is display metadata, not identity.
func (c *Client) ValidateConnectionName() error {
	if c.ConnectionName == nil {
		return nil
	}
	name := strings.TrimSpace(*c.ConnectionName)
	if utf8.RuneCountInString(name) > 256 || strings.ContainsFunc(name, unicode.IsControl) {
		return errors.New("connection name must be at most 256 characters without control characters")
	}
	c.ConnectionName = &name
	return nil
}

func (c Client) ConnectionDisplayName() string {
	if c.ConnectionName == nil {
		return ""
	}
	return strings.TrimSpace(*c.ConnectionName)
}
