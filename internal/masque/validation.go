package masque

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateUsername checks the HTTP Basic username before it reaches the runtime.
// A colon would be interpreted as the password separator (RFC 7617).
func ValidateUsername(username string) error {
	if strings.TrimSpace(username) == "" || strings.Contains(username, ":") {
		return fmt.Errorf("MASQUE requires a nonempty HTTP Basic username without a colon")
	}
	for _, ch := range username {
		if ch < 0x20 || ch == 0x7f {
			return fmt.Errorf("MASQUE username contains a control character")
		}
	}
	return nil
}

// ValidatePath accepts the path and query expression forms understood by
// sing-box's CONNECT-IP template parser. Unknown variables cannot be expanded
// by the client, so reject them instead of saving an unreachable listener.
func ValidatePath(path string) error {
	invalid := func() error { return fmt.Errorf("invalid MASQUE URI template path %q", path) }
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "#") {
		return invalid()
	}
	for _, ch := range path {
		if ch < 0x21 || ch > 0x7e {
			return invalid()
		}
	}
	remaining, inQuery := path, false
	var expanded strings.Builder
	for {
		open := strings.IndexByte(remaining, '{')
		if open < 0 {
			if strings.Contains(remaining, "}") {
				return invalid()
			}
			expanded.WriteString(remaining)
			break
		}
		literal := remaining[:open]
		close := strings.IndexByte(remaining[open:], '}')
		if close < 0 || strings.Contains(literal, "}") {
			return invalid()
		}
		body := remaining[open+1 : open+close]
		remaining = remaining[open+close+1:]
		if body == "" {
			return invalid()
		}
		operator := byte(0)
		if body[0] == '?' || body[0] == '&' {
			operator, body = body[0], body[1:]
		}
		variables := strings.Split(body, ",")
		for _, variable := range variables {
			if variable != "target" && variable != "ipproto" {
				return invalid()
			}
		}
		inQuery = inQuery || strings.Contains(literal, "?") || operator != 0
		if operator == 0 {
			if len(variables) != 1 {
				return invalid()
			}
			if inQuery {
				key := literal[strings.LastIndexAny(literal, "?&")+1:]
				if len(key) < 2 || !strings.HasSuffix(key, "=") {
					return invalid()
				}
			}
		}
		expanded.WriteString(literal)
		for _, variable := range variables {
			if operator != 0 {
				expanded.WriteByte(operator)
				expanded.WriteString(variable + "=")
				operator = '&'
			}
			expanded.WriteByte('*')
		}
	}
	if _, err := url.ParseRequestURI(expanded.String()); err != nil {
		return invalid()
	}
	return nil
}
