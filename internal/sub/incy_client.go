package sub

import "regexp"

// INCY clients identify themselves as INCY/<version>/<platform>.
var incyUserAgentRegex = regexp.MustCompile(`(?i)\bincy\b`)

// IsIncyClient checks whether the subscription belongs to INCY.
func IsIncyClient(userAgent string) bool {
	return incyUserAgentRegex.MatchString(userAgent)
}
