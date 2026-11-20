package zapret2

import (
 "bytes"
 "strings"
 "sync"
 "testing"
)

func TestCappedLogPreservesSummaryAtEnd(t *testing.T){
 var log cappedLog
 prefix:=bytes.Repeat([]byte{'x'},(1<<20)+900)
 if n,err:=log.Write(prefix);err!=nil||n!=len(prefix){t.Fatalf("write failed: %v %d",err,n)}
 tail:="\n* SUMMARY\n--lua-desync=multisplit:pos=1\n"
 if n,err:=log.Write([]byte(tail));err!=nil||n!=len(tail){t.Fatalf("tail write failed: %v %d",err,n)}
 got:=log.String()
 if len(got)>1<<20||!strings.HasSuffix(got,tail){t.Fatalf("missing trailing blockcheck summary, len=%d",len(got))}
}

func TestCappedLogConcurrentWrites(t *testing.T){
 var log cappedLog
 var group sync.WaitGroup
 for i:=0;i<12;i++{
  group.Add(1)
  go func(){defer group.Done();for n:=0;n<300;n++{_,_=log.Write([]byte("strategy attempt\n"))}}()
 }
 group.Wait()
 if len(log.String())>1<<20{t.Fatal("diagnostic buffer escaped size cap")}
}

func TestRejectPrivateIPAddressAsDiagnosticDomain(t *testing.T) {
 for _,domain:=range []string{"192.168.0.1","127.0.0.1","localhost",".local","8.8.8.8"} {
  if ValidateDiagnosticsDomains([]string{domain})==nil {t.Fatalf("unexpected private address accepted: %s",domain)}
 }
 if err:=ValidateDiagnosticsDomains([]string{"example.com"});err!=nil{t.Fatal(err)}
}
