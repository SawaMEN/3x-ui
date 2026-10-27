package model

import (
	"errors"
	"strings"
)

const (
	ConnectionNameRemark       = "remark"
	ConnectionNameRemarkClient = "remark-client"
	ConnectionNameClient       = "client"
)

// Omission preserves the saved mode and older callers' naming templates.
func (c *Client) ValidateConnectionNameMode() error {
	if c.ConnectionNameMode == nil {
		return nil
	}
	switch *c.ConnectionNameMode {
	case ConnectionNameRemark, ConnectionNameRemarkClient, ConnectionNameClient:
		return nil
	default:
		return errors.New("connection name mode must be remark, remark-client, or client")
	}
}

// Client means its email identifier. A blank remark falls back to that identifier
// in both remark modes, keeping names usable and avoiding a leading separator.
func (c Client) ConnectionDisplayName(remark string) string {
	if c.ConnectionNameMode == nil {
		return ""
	}
	email := strings.TrimSpace(c.Email)
	remark = strings.TrimSpace(remark)
	switch *c.ConnectionNameMode {
	case ConnectionNameClient:
		return email
	case ConnectionNameRemark:
		if remark != "" {
			return remark
		}
		return email
	case ConnectionNameRemarkClient:
		if remark != "" && email != "" {
			return remark + "-" + email
		}
		if remark != "" {
			return remark
		}
		return email
	default:
		return ""
	}
}
