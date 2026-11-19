package zapret2

import (
 "net"
 "sync"
 "context"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
 "strings"
 "time"
)

type DiagnosticResult struct {
 BestProfile string `json:"bestProfile"`
 Summary string `json:"summary"`
 Domains []string `json:"domains"`
 DurationSeconds float64 `json:"durationSeconds"`
}
var diagnosticsHost = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$`)

func ValidateDiagnosticsDomains(domains []string) error {
 if len(domains)==0 || len(domains)>12 {return fmt.Errorf("provide 1..12 public hostnames for strategy diagnostics")}
 for _,domain:=range domains{
  domain=strings.TrimSpace(domain)
  if !diagnosticsHost.MatchString(domain)||!strings.Contains(domain,".")||net.ParseIP(domain)!=nil||
   strings.EqualFold(domain,"localhost")||strings.HasSuffix(strings.ToLower(domain),".local")||
   strings.HasSuffix(strings.ToLower(domain),".internal") {
    return fmt.Errorf("invalid public test domain %q",domain)
  }
 }
 return nil
}

var diagnosticsMu sync.Mutex

func RunBlockcheck(ctx context.Context,domains []string) (DiagnosticResult,error) {
 if !diagnosticsMu.TryLock() {return DiagnosticResult{},fmt.Errorf("a zapret2 diagnostic is already running")}
 defer diagnosticsMu.Unlock()
 if !Installed(){return DiagnosticResult{},fmt.Errorf("install the official zapret2 component first")}
 if os.Geteuid()!=0{return DiagnosticResult{},fmt.Errorf("blockcheck2 requires root")}
 if err:=ValidateDiagnosticsDomains(domains);err!=nil{return DiagnosticResult{},err}
 for i:=range domains{domains[i]=strings.TrimSpace(domains[i])}
 scratch,err:=os.MkdirTemp("","3x-ui-zapret2-blockcheck-*")
 if err!=nil{return DiagnosticResult{},err}
 defer os.RemoveAll(scratch)
 script:=filepath.Join(Directory(),"blockcheck2.sh")
 if _,err:=os.Stat(script);err!=nil{return DiagnosticResult{},err}
 ctx,cancel:=context.WithTimeout(ctx,8*time.Minute)
 defer cancel()
 cmd:=exec.CommandContext(ctx,"/bin/sh",script)
 cmd.Dir=Directory()
 cmd.Env=append(os.Environ(),
  "BATCH=1",
  "TEST=standard",
  "DOMAINS="+strings.Join(domains," "),
  "IPVS=4",
  "ZAPRET_BASE="+Directory(),
  "ZAPRET_RW="+scratch,
  "NFQWS2="+BinaryPath(),
 )
 // Bound log collection: blockcheck can emit thousands of strategy attempts.
 var out cappedLog
 cmd.Stdout=&out;cmd.Stderr=&out
 start:=time.Now()
 err=cmd.Run()
 full:=out.String()
 summary:=full
 if pos:=strings.LastIndex(full,"* SUMMARY");pos>=0{summary=full[pos:]}
 if len(summary)>14000{summary=summary[len(summary)-14000:]}
 result:=DiagnosticResult{Summary:summary,Domains:domains,DurationSeconds:time.Since(start).Seconds()}
 // Use only success summaries. Do not infer a 'winner' from unsuccessful
 // intermediate attempts. A summary without recognized patterns is unranked.
 if strings.Contains(full,"* SUMMARY") {
  // Upstream reports successful strategies in SUMMARY (unsuccessful attempts
  // occur earlier in the log). Match an entire known preset signature rather
  // than guessing from one word that happens to occur in a different recipe.
  // If no exact equivalent exists, return the report without a false winner.
  summaryPart:=summary
  if i:=strings.Index(summaryPart,"* COVERAGE");i>=0 {
   summaryPart=summaryPart[i:]
  } else if i:=strings.Index(summaryPart,"* COMMON");i>=0 {
   summaryPart=summaryPart[i:]
  }
  for _,line:=range strings.Split(summaryPart,"\n"){
   if strings.Contains(line,"not working")||!strings.Contains(line,"--lua-desync="){continue}
   switch {
   case strings.Contains(line,"--lua-desync=fake:blob=fake_default_tls:tcp_md5:tls_mod=rnd,rndsni,dupsid") &&
      strings.Contains(line,"--lua-desync=multidisorder:pos=1,midsld"):
      result.BestProfile="aggressive"
   case strings.Contains(line,"--lua-desync=fake:blob=fake_default_tls:tcp_md5:tcp_seq=-10000") &&
      strings.Contains(line,"--lua-desync=multidisorder:pos=1,midsld"):
      result.BestProfile="balanced"
   case strings.Contains(line,"--lua-desync=multisplit:pos=1") &&
      !strings.Contains(line,"--lua-desync=fake"):
      result.BestProfile="safe"
   }
   if result.BestProfile!=""{break}
  }
 }
 if err!=nil{
  if ctx.Err()!=nil{return result,fmt.Errorf("zapret2 blockcheck cancelled or timed out: %w",ctx.Err())}
  return result,fmt.Errorf("official blockcheck2 failed: %w",err)
 }
 return result,nil
}

// blockcheck emits its useful SUMMARY at the END of a potentially verbose
// scan. Retain the bounded tail, not the beginning. Stdout and stderr can be
// copied concurrently by os/exec, so writes must be serialized.
type cappedLog struct {
 mu sync.Mutex
 data []byte
}
func (w *cappedLog) Write(p []byte)(int,error) {
 w.mu.Lock()
 defer w.mu.Unlock()
 const capBytes=1<<20
 original:=len(p)
 if original>=capBytes {
  w.data=append(w.data[:0],p[original-capBytes:]...)
  return original,nil
 }
 if over:=len(w.data)+original-capBytes;over>0 {
  copy(w.data,w.data[over:])
  w.data=w.data[:len(w.data)-over]
 }
 w.data=append(w.data,p...)
 return original,nil
}
func (w *cappedLog) String()string{
 w.mu.Lock()
 defer w.mu.Unlock()
 return string(w.data)
}
