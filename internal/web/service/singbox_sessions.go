package service

import (
	"context"
	"errors"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// ActiveSessions returns the current sing-box connection snapshot without
// sharing state with the traffic accounting poller.
func (s *SingBoxService) ActiveSessions(ctx context.Context) ([]singbox.ActiveSession, error) {
	{
		return s.hiddifySessions(ctx)
	}

}

// DisconnectUserSessions closes every currently tracked connection belonging
// to user. inbound can be empty to match the user across all inbounds.
func (s *SingBoxService) DisconnectUserSessions(ctx context.Context, inbound, user string) (int, error) {
	if user == "" {
		return 0, errors.New("user is required")
	}
	return s.DisconnectUsersSessions(ctx, inbound, []string{user})
}

// DisconnectUsersSessions closes currently tracked connections for all supplied
// users using one native API snapshot. Empty user names are ignored.
func (s *SingBoxService) DisconnectUsersSessions(ctx context.Context, inbound string, users []string) (int, error) {
	hasUser := false
	for _, user := range users {
		if user != "" {
			hasUser = true
			break
		}
	}
	if !hasUser {
		return 0, errors.New("at least one user is required")
	}
	{
		return s.disconnectHiddify(ctx, func(session singbox.ActiveSession) bool {
			if inbound != "" && session.Inbound != inbound {
				return false
			}
			for _, user := range users {
				if user != "" && session.User == user {
					return true
				}
			}
			return false
		})
	}

}

// DisconnectInboundSessions closes every currently tracked connection for an
// inbound tag without changing that inbound's configuration.
func (s *SingBoxService) DisconnectInboundSessions(ctx context.Context, inbound string) (int, error) {
	if inbound == "" {
		return 0, errors.New("inbound is required")
	}
	{
		return s.disconnectHiddify(ctx, func(session singbox.ActiveSession) bool { return session.Inbound == inbound })
	}

}
