package service

import "testing"

func TestParseXrayAccessLogConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		want string
		bad  bool
	}{
		{name: "absolute path", data: `{"log":{"access":"/var/log/xray/access.log"}}`, want: "/var/log/xray/access.log"},
		{name: "disabled", data: `{"log":{"access":"none"}}`, want: "none"},
		{name: "missing", data: `{"log":{"loglevel":"warning"}}`, want: ""},
		{name: "malformed", data: `{"log":`, bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseXrayAccessLogConfig([]byte(tc.data))
			if (err != nil) != tc.bad {
				t.Fatalf("parse error = %v; expected error: %v", err, tc.bad)
			}
			if !tc.bad && got != tc.want {
				t.Fatalf("log path = %q; want %q", got, tc.want)
			}
		})
	}
}
