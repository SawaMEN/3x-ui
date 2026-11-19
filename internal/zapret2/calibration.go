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
	Kind     string   `json:"kind"`
	IP       string   `json:"ip"`
	Test     string   `json:"test"`
	Args     []string `json:"args"`
	Passed   int      `json:"passed"`
	Total    int      `json:"total"`
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

var coverageLine = regexp.MustCompile(`^(\d+)/(\d+)\s*:\s*(curl_test_(?:http|https_tls12|https_tls13|http3))\s+ipv([46])\s*:\s*nfqws2\s+(.+)$`)
var reportLine = regexp.MustCompile(`^(curl_test_(?:http|https_tls12|https_tls13|http3))\s+ipv([46])\s+(\S+)\s*:\s*nfqws2\s+(.+)$`)
var strategyValue = regexp.MustCompile(`^[A-Za-z0-9_+<>=:.,%-]{1,512}$`)

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
	for _,arg := range args {
		name,value,ok := strings.Cut(arg,"=")
		if !ok || !strings.HasPrefix(name,"--") || !strategyValue.MatchString(value) ||
			strings.ContainsAny(value, "@$\\`;/|&\n\r") {
			return fmt.Errorf("unsafe zapret2 strategy parameter %q",arg)
		}
		switch name {
		case "--lua-desync":
			hasLua=true
		case "--payload","--in-range","--out-range","--blob":
		default:
			return fmt.Errorf("unsupported zapret2 strategy parameter %q",name)
		}
	}
	if !hasLua {
		return errors.New("strategy does not include --lua-desync")
	}
	return nil
}

func ValidateCalibration(c *Calibration) error {
	if c == nil { return nil }
	if c.Source != "blockcheck2" { return errors.New("unsupported calibration source") }
	if c.Level != "quick" && c.Level != "standard" && c.Level != "force" { return errors.New("unsupported scan level") }
	if _,err:=time.Parse(time.RFC3339,c.TestedAt);err!=nil{return fmt.Errorf("invalid calibration timestamp: %w",err)}
	if err:=ValidateDiagnosticsDomains(c.Domains);err!=nil{return err}
	if len(c.Selected)<1 || len(c.Selected)>6 {return errors.New("calibration must include 1..6 strategies")}
	seen:=map[string]bool{}
	for _,s:=range c.Selected {
		if s.Kind != "http" && s.Kind != "https" && s.Kind != "quic" {return fmt.Errorf("invalid strategy kind %q",s.Kind)}
		if s.IP != "4" && s.IP != "6" {return fmt.Errorf("invalid strategy IP family %q",s.IP)}
		if kindForTest(s.Test)!=s.Kind {return fmt.Errorf("test %q is not for %s",s.Test,s.Kind)}
		key:=s.Kind+"_"+s.IP
		if seen[key]{return fmt.Errorf("duplicate calibrated strategy for %s",key)}
		seen[key]=true
		if s.Total!=len(c.Domains) || s.Passed<1 || s.Passed>s.Total || s.Passed*3<s.Total*2 {return fmt.Errorf("insufficient or invalid calibration coverage for %s",key)}
		if err:=validateStrategyArgs(s.Args);err!=nil{return err}
	}
	return nil
}

// ParseCalibrationSummary consumes the authoritative final SUMMARY output,
// never mid-run "working" lines, and does not mistake "not working" entries for
// a successful strategy. COVERAGE is ranked for multi-domain tests; the
// per-domain SUMMARY is supported for a single domain.
func ParseCalibrationSummary(summary string,domains []string,level string) (*Calibration,error) {
	if err:=ValidateDiagnosticsDomains(domains);err!=nil{return nil,err}
	if level!="quick" && level!="standard" && level!="force"{return nil,fmt.Errorf("invalid scan level %q",level)}
	idx:=strings.LastIndex(summary,"* SUMMARY")
	if idx<0{return nil,errors.New("blockcheck2 did not produce a final SUMMARY")}
	summary=summary[idx+len("* SUMMARY"):]
	lines:=strings.Split(summary,"\n")
	inCoverage:=false
	var results []SelectedStrategy
	for _,line:=range lines {
		line=strings.TrimSpace(line)
		if strings.HasPrefix(line,"* "){
			inCoverage = strings.HasPrefix(line,"* COVERAGE")
			continue
		}
		if line=="" || strings.Contains(line,"not working") {continue}
		var test,ip,argText string
		passed,total:=0,len(domains)
		if inCoverage && len(domains)>1 {
			match:=coverageLine.FindStringSubmatch(line)
			if match==nil{continue}
			passed,_=strconv.Atoi(match[1])
			total,_=strconv.Atoi(match[2])
			test,ip,argText=match[3],match[4],match[5]
		} else if len(domains)==1 && !inCoverage {
			match:=reportLine.FindStringSubmatch(line)
			if match==nil{continue}
			passed=1
			test,ip,argText=match[1],match[2],match[4]
		} else {continue}
		if total!=len(domains)||passed<1||passed>total{continue}
		// Require at least two thirds of the tested targets to work before
		// promoting a recipe to active routing. Partial success below this
		// threshold stays visible in the raw blockcheck2 report.
		if passed*3<total*2 {continue}
		// Only accept the token list after the daemon name. Blockcheck may
		// include external/untrusted file paths: validation rejects those.
		args:=strings.Fields(argText)
		if validateStrategyArgs(args)!=nil {continue}
		results=append(results,SelectedStrategy{
			Kind:kindForTest(test),IP:ip,Test:test,Args:args,Passed:passed,Total:total,
		})
	}
	if len(results)==0{return nil,errors.New("no safe, successful nfqws2 strategies in blockcheck2 SUMMARY")}
	sort.SliceStable(results,func(i,j int)bool{
		a,b:=results[i],results[j]
		if a.Passed!=b.Passed{return a.Passed>b.Passed}
		if a.Test!=b.Test && a.Kind=="https" && b.Kind=="https" {
			return a.Test=="curl_test_https_tls12"
		}
		if len(a.Args)!=len(b.Args){return len(a.Args)<len(b.Args)}
		return strings.Join(a.Args," ")<strings.Join(b.Args," ")
	})
	seen:=map[string]bool{}
	selected:=make([]SelectedStrategy,0,6)
	for _,s:=range results {
		key:=s.Kind+"_"+s.IP
		if seen[key]{continue}
		seen[key]=true
		selected=append(selected,s)
	}
	calibration:=&Calibration{
		Source:"blockcheck2",TestedAt:time.Now().UTC().Format(time.RFC3339),
		Domains:append([]string(nil),domains...),Level:level,Selected:selected,
	}
	if err:=ValidateCalibration(calibration);err!=nil{return nil,err}
	return calibration,nil
}

// CalibratedArgs returns only a verified, saved protocol/family match.
// Missing categories fall back to the built-in mode rather than silently
// trying a candidate tested against a different protocol.
func CalibratedArgs(c *Calibration,kind,ip string) []string {
	if c==nil{return nil}
	for _,s:=range c.Selected {
		if s.Kind==kind && s.IP==ip{return append([]string(nil),s.Args...)}
	}
	return nil
}
