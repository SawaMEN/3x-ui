package service

import (
	"net/http"
	"testing"
)

func TestTelemtWebHealthyDecoyStatus(t *testing.T) {
	for _, tc := range []struct{ status int; want bool }{
		{http.StatusOK, true},
		{http.StatusNoContent, true},
		{http.StatusMovedPermanently, true},
		{http.StatusFound, true},
		{http.StatusUnauthorized, true},
		{http.StatusForbidden, true},
		{http.StatusNotFound, true},
		{http.StatusMethodNotAllowed, true},
		{http.StatusBadRequest, false},
		{http.StatusTooManyRequests, false},
		{http.StatusInternalServerError, false},
		{http.StatusBadGateway, false},
		{http.StatusServiceUnavailable, false},
	} {
		if got := telemtWebHealthyDecoyStatus(tc.status); got != tc.want {
			t.Errorf("HTTP %d: healthy=%t want %t", tc.status, got, tc.want)
		}
	}
}
