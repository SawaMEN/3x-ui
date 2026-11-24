package service

import (
	"slices"
	"strings"
)

// nginx -T prints separate configuration files, each with a marker.
// Only foreign TCP listen directives prevent takeover of TCP 443. UDP/QUIC
// can coexist; comments, quoted values and multiline directives are allowed.
func nginxHasForeignTCP443(dump string, ownFiles ...string) bool {
	file := ""
	var fields []string
	var token strings.Builder
	var quote rune
	escaped := false
	active := false
	flush := func() {
		if token.Len() != 0 {
			fields = append(fields, token.String())
			token.Reset()
		}
	}
	for _, line := range strings.Split(dump, "\n") {
		trimmed := strings.TrimSpace(line)
		if quote == 0 && strings.HasPrefix(trimmed, "# configuration file ") {
			file = strings.TrimSuffix(strings.TrimPrefix(trimmed, "# configuration file "), ":")
			active = !slices.Contains(ownFiles, file)
			fields = nil
			token.Reset()
			escaped = false
			continue
		}
		if !active {
			continue
		}
		for _, r := range line + "\n" {
			if escaped {
				token.WriteRune(r)
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if quote != 0 {
				if r == quote {
					quote = 0
				} else {
					token.WriteRune(r)
				}
				continue
			}
			switch r {
			case '"', '\'':
				quote = r
			case '#':
				flush()
				// Ignore the rest of the comment, not pending arguments.
				goto nextLine
			case ' ', '\t', '\r', '\n':
				flush()
			case ';', '{', '}':
				flush()
				if r == ';' && len(fields) >= 2 && fields[0] == "listen" &&
					(fields[1] == "443" || strings.HasSuffix(fields[1], ":443")) &&
					!slices.Contains(fields[2:], "quic") && !slices.Contains(fields[2:], "udp") {
					return true
				}
				fields = nil
			default:
				token.WriteRune(r)
			}
		}
	nextLine:
	}
	return false
}
