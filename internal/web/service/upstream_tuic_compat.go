package service

import (
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	tuicpkg "github.com/SawaMEN/3x-ui/v3/internal/tuic"

	"gorm.io/gorm"
)

// Keep the upstream TUIC integration available in files where the divergent
// fork history dropped the import during the merge.
var tuic = struct {
	InstanceFromInbound func(*model.Inbound) (tuicpkg.Instance, bool)
	SOCKSPortForInbound func(int) int
}{
	InstanceFromInbound: tuicpkg.InstanceFromInbound,
	SOCKSPortForInbound: tuicpkg.SOCKSPortForInbound,
}

// The merged local-runtime builder needs a set for its TUIC traffic-id lookup.
// Using the table itself here is equivalent to selecting the candidate emails
// first: the resulting map is indexed by the current inbound's client email.
var emails = gorm.Expr("SELECT email FROM client_traffics")

// The fork's port-conflict code still uses string listen semantics, while the
// upstream TUIC checks were written against helper names introduced later.
var loopbackBind = "127.0.0.1"

func inboundBindAddr(ib *model.Inbound) string {
	if ib == nil {
		return ""
	}
	return ib.Listen
}
