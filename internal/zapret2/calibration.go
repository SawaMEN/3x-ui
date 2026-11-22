package zapret2

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SelectedStrategy is an exact nfqws2 strategy recorded by upstream blockcheck2.
// The command is stored as a tokenized argument list, never a shell command.
type SelectedStrategy struct {
	Kind   string   `json:"kind"`
	IP     string   `json:"ip"`
	Test   string   `json:"test"`
	Args   []string `json:"args"`
	Passed int      `json:"passed"`
	Total  int      `json:"total"`
}

// Calibration travels in the individual outbound's settings. Nothing is
// globally activated; changes only take effect on core restart/apply.
type Calibration struct {
	Source   string             `json:"source"`
	TestedAt string             `json:"testedAt"`
	Domains  []string           `json:"domains"`
	Level    string             `json:"level"`
	Selected []SelectedStrategy `json:"selected"`
}

var (
	coverageLine  = regexp.MustCompile(`^(\d+)/(\d+)\s*:\s*(curl_test_(?:http|https_tls12|https_tls13|http3))\s+ipv([46])\s*:\s*nfqws2\s+(.+)$`)
	reportLine    = regexp.MustCompile(`^(curl_test_(?:http|https_tls12|https_tls13|http3))\s+ipv([46])\s+(\S+)\s*:\s*nfqws2\s+(.+)$`)
	baselineLine  = regexp.MustCompile(`^(curl_test_(?:http|https_tls12|https_tls13|http3))\s+ipv([46])\s+(\S+)\s*:\s*working without bypass$`)
	strategyValue = regexp.MustCompile(`^[A-Za-z0-9_+<>=:.,%-]{1,512}$`)
)

func kindForTest(test string) string {
	switch test {
	case "curl_test_http":
		return "http"
	case "curl_test_https_tls12", "curl_test_https_tls13":
		return "https"
	case "curl_test_http3":
		return "quic"
	default:
		return ""
	}
}

// validateStrategyArgs rejects file references, shell words and process-level
// switches. blockcheck2 can emit different options between releases, so do
// not apply a command unless EVERY token belongs to a supported safe subset.
func validateStrategyArgs(args []string) error {
	if len(args) == 0 || len(args) > 24 {
		return errors.New("strategy must contain 1..24 options")
	}
	var hasLua bool
	for _, arg := range args {
		name, value, ok := strings.Cut(arg, "=")
		if !ok || !strings.HasPrefix(name, "--") || !strategyValue.MatchString(value) ||
			strings.ContainsAny(value, "@$\\`;/|&\n\r") {
			return fmt.Errorf("unsafe zapret2 strategy parameter %q", arg)
		}
		switch name {
		case "--lua-desync":
			hasLua = true
		case "--payload", "--in-range", "--out-range", "--blob":
		default:
			return fmt.Errorf("unsupported zapret2 strategy parameter %q", name)
		}
	}
	if !hasLua {
		return errors.New("strategy does not include --lua-desync")
	}
	return nil
}

func ValidateCalibration(c *Calibration) error {
	if c == nil {
		return nil
	}
	if c.Source != "blockcheck2" {
		return errors.New("unsupported calibration source")
	}
	if c.Level != "quick" && c.Level != "standard" && c.Level != "force" {
		return errors.New("unsupported scan level")
	}
	if _, err := time.Parse(time.RFC3339, c.TestedAt); err != nil {
		return fmt.Errorf("invalid calibration timestamp: %w", err)
	}
	if err := ValidateDiagnosticsDomains(c.Domains); err != nil {
		return err
	}
	if len(c.Selected) < 1 || len(c.Selected) > 6 {
		return errors.New("calibration must include 1..6 strategies")
	}
	seen := map[string]bool{}
	for _, s := range c.Selected {
		if s.Kind != "http" && s.Kind != "https" && s.Kind != "quic" {
			return fmt.Errorf("invalid strategy kind %q", s.Kind)
		}
		if s.IP != "4" && s.IP != "6" {
			return fmt.Errorf("invalid strategy IP family %q", s.IP)
		}
		if kindForTest(s.Test) != s.Kind {
			return fmt.Errorf("test %q is not for %s", s.Test, s.Kind)
		}
		key := s.Kind + "_" + s.IP
		if seen[key] {
			return fmt.Errorf("duplicate calibrated strategy for %s", key)
		}
		seen[key] = true
		if (s.Total < 1 || s.Total > len(c.Domains)) || s.Passed < 1 || s.Passed > s.Total || s.Passed*3 < s.Total*2 {
			return fmt.Errorf("insufficient or invalid calibration coverage for %s", key)
		}
		if err := validateStrategyArgs(s.Args); err != nil {
			return err
		}
	}
	return nil
}

// ParseCalibrationSummary only accepts successful exact nfqws2 options from
// the final official blockcheck2 report. Baseline successes are excluded from
// the score: passing an already-unblocked site is not proof of DPI bypass.
func ParseCalibrationSummary(summary string, domains []string, level string) (*Calibration, error) {
	if err := ValidateDiagnosticsDomains(domains); err != nil {
		return nil, err
	}
	if level != "quick" && level != "standard" && level != "force" {
		return nil, fmt.Errorf("invalid scan level %q", level)
	}
	idx := strings.LastIndex(summary, "* SUMMARY")
	if idx < 0 {
		return nil, errors.New("blockcheck2 did not produce a final SUMMARY")
	}
	summary = summary[idx+len("* SUMMARY"):]
	lines := strings.Split(summary, "\n")
	domainSet := make(map[string]bool, len(domains))
	for _, d := range domains {
		domainSet[d] = true
	}
	// Key = test + IP version. Real, successful domain reports are a
	// higher-quality signal than COVERAGE counts when some targets already
	// work without bypass.
	baselines := map[string]map[string]bool{}
	hits := map[string]map[string]bool{}
	keyFor := func(test, ip string) string { return test + "|" + ip }
	signature := func(test, ip, stringArgs string) string {
		return test + "|" + ip + "|" + strings.Join(strings.Fields(stringArgs), " ")
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if m := baselineLine.FindStringSubmatch(line); m != nil {
			if domainSet[m[3]] {
				key := keyFor(m[1], m[2])
				if baselines[key] == nil {
					baselines[key] = map[string]bool{}
				}
				baselines[key][m[3]] = true
			}
			continue
		}
		if m := reportLine.FindStringSubmatch(line); m != nil {
			if !domainSet[m[3]] {
				continue
			}
			key := signature(m[1], m[2], m[4])
			if hits[key] == nil {
				hits[key] = map[string]bool{}
			}
			hits[key][m[3]] = true
		}
	}
	var candidates []SelectedStrategy
	consider := func(test, ip, argText string, covered, total int) {
		args := strings.Fields(argText)
		if validateStrategyArgs(args) != nil || covered < 1 || total != len(domains) {
			return
		}
		baseline := baselines[keyFor(test, ip)]
		if len(baseline) == len(domains) {
			return
		} // No bypass was necessary.
		passed, eligible := covered, total
		if len(baseline) > 0 {
			eligible = len(domains) - len(baseline)
			passed = 0
			for d := range hits[signature(test, ip, argText)] {
				if !baseline[d] {
					passed++
				}
			}
			// A candidate that only worked on already-unblocked targets
			// must never become a recommended automatic strategy.
			if passed == 0 {
				return
			}
		}
		if passed < 1 || eligible < 1 || passed > eligible || passed*3 < eligible*2 {
			return
		}
		candidates = append(candidates, SelectedStrategy{
			Kind: kindForTest(test), IP: ip, Test: test,
			Args: args, Passed: passed, Total: eligible,
		})
	}
	inCoverage := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "* ") {
			inCoverage = strings.HasPrefix(line, "* COVERAGE")
			continue
		}
		if line == "" || strings.Contains(line, "not working") {
			continue
		}
		if inCoverage && len(domains) > 1 {
			if m := coverageLine.FindStringSubmatch(line); m != nil {
				passed, _ := strconv.Atoi(m[1])
				total, _ := strconv.Atoi(m[2])
				consider(m[3], m[4], m[5], passed, total)
			}
		} else if len(domains) == 1 && !inCoverage {
			if m := reportLine.FindStringSubmatch(line); m != nil {
				if domainSet[m[3]] {
					consider(m[1], m[2], m[4], 1, 1)
				}
			}
		}
	}
	if len(candidates) == 0 {
		return nil, errors.New("no sufficiently successful nfqws2 strategy for sites that need DPI bypass")
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.Passed*b.Total != b.Passed*a.Total {
			return a.Passed*b.Total > b.Passed*a.Total
		}
		if a.Passed != b.Passed {
			return a.Passed > b.Passed
		}
		if a.Test != b.Test && a.Kind == "https" && b.Kind == "https" {
			return a.Test == "curl_test_https_tls12"
		}
		if len(a.Args) != len(b.Args) {
			return len(a.Args) < len(b.Args)
		}
		return strings.Join(a.Args, " ") < strings.Join(b.Args, " ")
	})
	seen := map[string]bool{}
	selected := make([]SelectedStrategy, 0, 6)
	for _, s := range candidates {
		key := s.Kind + "_" + s.IP
		if seen[key] {
			continue
		}
		seen[key] = true
		selected = append(selected, s)
	}
	calibration := &Calibration{
		Source: "blockcheck2", TestedAt: time.Now().UTC().Format(time.RFC3339),
		Domains: append([]string(nil), domains...), Level: level, Selected: selected,
	}
	if err := ValidateCalibration(calibration); err != nil {
		return nil, err
	}
	return calibration, nil
}

// CalibratedArgs returns only a verified, saved protocol/family match.
// Missing categories fall back to the built-in mode rather than silently
// trying a candidate tested against a different protocol.
func CalibratedArgs(c *Calibration, kind, ip string) []string {
	if c == nil {
		return nil
	}
	for _, s := range c.Selected {
		if s.Kind == kind && s.IP == ip {
			return append([]string(nil), s.Args...)
		}
	}
	return nil
}
