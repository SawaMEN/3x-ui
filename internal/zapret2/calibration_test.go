package zapret2

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseCalibrationSummaryUsesCoverageRatherThanFirstHit(t *testing.T) {
	log := `attempt: curl_test_https_tls12 ipv4 example.com --lua-desync=bad
* SUMMARY
curl_test_https_tls12 ipv4 example.com : nfqws2 --payload=tls_client_hello --lua-desync=multisplit:pos=1
curl_test_https_tls12 ipv4 example.com : nfqws2 not working
* COMMON
* COVERAGE
1/3 : curl_test_https_tls12 ipv4 : nfqws2 --payload=tls_client_hello --lua-desync=multisplit:pos=2
3/3 : curl_test_https_tls12 ipv4 : nfqws2 --payload=tls_client_hello --lua-desync=multisplit:pos=1
2/3 : curl_test_https_tls12 ipv4 : nfqws2 --payload=tls_client_hello --lua-desync=fake:blob=fake_default_tls:tcp_md5
3/3 : curl_test_http ipv4 : nfqws2 --payload=http_req --lua-desync=http_hostcase
3/3 : curl_test_http3 ipv4 : nfqws2 --payload=quic_initial --lua-desync=fake:blob=fake_default_quic:repeats=5
`
	calibration,err:=ParseCalibrationSummary(log,[]string{"example.com","example.org","example.net"},"force")
	if err!=nil{t.Fatal(err)}
	if len(calibration.Selected)!=3{t.Fatalf("expected HTTP/HTTPS/QUIC, got %#v",calibration.Selected)}
	want:=[]string{"--payload=tls_client_hello","--lua-desync=multisplit:pos=1"}
	if !reflect.DeepEqual(CalibratedArgs(calibration,"https","4"),want){
		t.Fatalf("winner should be best coverage and deterministic tie-break: %+v",CalibratedArgs(calibration,"https","4"))
	}
	for _,s:=range calibration.Selected {
		if s.Passed!=3 || s.Total!=3 {t.Errorf("coverage lost: %+v",s)}
	}
}

func TestSingleDomainSummaryAndTLS13Fallback(t *testing.T) {
	log:=`header
* SUMMARY
curl_test_https_tls13 ipv6 example.com : nfqws2 --payload=tls_client_hello --lua-desync=multidisorder:pos=1,midsld
curl_test_http ipv4 example.com : nfqws2 not working
`
	c,err:=ParseCalibrationSummary(log,[]string{"example.com"},"standard")
	if err!=nil{t.Fatal(err)}
	if len(c.Selected)!=1||c.Selected[0].IP!="6"||c.Selected[0].Test!="curl_test_https_tls13" {
		t.Fatalf("unexpected calibration: %#v",c)
	}
	if len(CalibratedArgs(c,"http","4"))!=0 {t.Fatal("non-working baseline was interpreted as a winner")}
}

func TestCalibrationRejectsUnsafeOrUnverifiedCommands(t *testing.T) {
	tests:=[][]string{
		{"--lua-desync=multisplit:pos=1","--hostlist=@/tmp/targets"},
		{"--lua-desync=multisplit:pos=1;rm-rf"},
		{"--lua-desync=multisplit:pos=1","--qnum=2"},
		{"--lua-desync=multisplit:pos=1","--blob=@/etc/passwd"},
		{"--payload=tls_client_hello"},
	}
	for _,args:=range tests {
		if err:=validateStrategyArgs(args);err==nil{
			t.Errorf("accepted untrusted strategy %#v",args)
		}
	}
	if err:=validateStrategyArgs([]string{"--payload=tls_client_hello","--lua-desync=multisplit:pos=1,midsld","--out-range=<s1"});err!=nil{
		t.Fatalf("rejected valid official positional argument: %v",err)
	}
	if _,err:=ParseCalibrationSummary("* SUMMARY\ncurl_test_http ipv4 example.com : nfqws2 --hostlist=@/tmp/a --lua-desync=multisplit:pos=1",[]string{"example.com"},"standard");err==nil{
		t.Fatal("untrusted calibration was returned as successful")
	}
}

func TestCalibrationAppliedOnlyOnAutoProfile(t *testing.T) {
	c:=&Calibration{
		Source:"blockcheck2",TestedAt:time.Now().UTC().Format(time.RFC3339),
		Domains:[]string{"example.com"},Level:"standard",
		Selected:[]SelectedStrategy{
			{Kind:"https",IP:"4",Test:"curl_test_https_tls12",Args:[]string{"--payload=tls_client_hello","--lua-desync=multisplit:pos=1,midsld"},Passed:1,Total:1},
			{Kind:"quic",IP:"6",Test:"curl_test_http3",Args:[]string{"--payload=quic_initial","--lua-desync=fake:blob=fake_default_quic:repeats=5"},Passed:1,Total:1},
		},
	}
	settings:=map[string]any{"port":15910,"profile":"auto","tcp":true,"udp":true,"calibration":c}
	p,err:=Parse("auto-1",settings)
	if err!=nil{t.Fatal(err)}
	args,err:=strategyArgs(p)
	if err!=nil{t.Fatal(err)}
	joined:=strings.Join(args," ")
	for _,text:=range []string{"--filter-l3=ipv4","--filter-l3=ipv6","--filter-tcp=443","--filter-tcp=80","--filter-udp=443","--lua-desync=multisplit:pos=1,midsld","--lua-desync=fake:blob=fake_default_quic:repeats=5"}{
		if !strings.Contains(joined,text){t.Fatalf("missing %q in calibrated args: %s",text,joined)}
	}
	if strings.Count(joined,"--new")!=5{t.Fatalf("expected six scoped nfqws2 groups, got: %s",joined)}
	settings["profile"]="safe"
	p,err=Parse("manual",settings)
	if err!=nil{t.Fatal(err)}
	manual,err:=strategyArgs(p)
	if err!=nil{t.Fatal(err)}
	if strings.Contains(strings.Join(manual," "),"--filter-l3=ipv4") {
		t.Fatalf("manually selected profile must override saved calibration: %v",manual)
	}
}

func TestCalibrationRejectsTamperedCoverage(t *testing.T) {
	c:=&Calibration{
		Source:"blockcheck2",TestedAt:time.Now().UTC().Format(time.RFC3339),
		Domains:[]string{"example.com"},Level:"quick",
		Selected:[]SelectedStrategy{
			{Kind:"https",IP:"4",Test:"curl_test_https_tls12",Args:[]string{"--lua-desync=multisplit:pos=1"},Passed:2,Total:1},
		},
	}
	if err:=ValidateCalibration(c);err==nil{t.Fatal("accepted impossible coverage")}
}


func TestCalibrationExcludesAlreadyUnblockedTargets(t *testing.T) {
	report := `* SUMMARY
curl_test_https_tls12 ipv4 example.com : working without bypass
curl_test_https_tls12 ipv4 example.com : nfqws2 --payload=tls_client_hello --lua-desync=multisplit:pos=1
curl_test_https_tls12 ipv4 example.org : nfqws2 --payload=tls_client_hello --lua-desync=multisplit:pos=1
curl_test_https_tls12 ipv4 example.org : nfqws2 --payload=tls_client_hello --lua-desync=multisplit:pos=2
curl_test_https_tls12 ipv4 example.net : nfqws2 --payload=tls_client_hello --lua-desync=multisplit:pos=2
* COVERAGE
2/3 : curl_test_https_tls12 ipv4 : nfqws2 --payload=tls_client_hello --lua-desync=multisplit:pos=1
2/3 : curl_test_https_tls12 ipv4 : nfqws2 --payload=tls_client_hello --lua-desync=multisplit:pos=2
`
	c,err:=ParseCalibrationSummary(report,[]string{"example.com","example.org","example.net"},"force")
	if err!=nil{t.Fatal(err)}
	s:=c.Selected[0]
	if s.Passed!=2||s.Total!=2||strings.Join(s.Args," ")!="--payload=tls_client_hello --lua-desync=multisplit:pos=2" {
		t.Fatalf("did not choose recipe covering both blocked targets: %+v",s)
	}
}

func TestCalibrationDoesNotPretendToBypassAlreadyAccessibleSites(t *testing.T) {
	report:=`* SUMMARY
curl_test_https_tls12 ipv4 example.com : working without bypass
curl_test_https_tls12 ipv4 example.com : nfqws2 --payload=tls_client_hello --lua-desync=multisplit:pos=1
`
	if _,err:=ParseCalibrationSummary(report,[]string{"example.com"},"force");err==nil{
		t.Fatal("reported a bypass strategy for an unblocked site")
	}
}
