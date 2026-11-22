package zapret2

import (
 "net"
 "sync"
 "syscall"
 "context"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
 "strings"
 "time"
)

type DiagnosticOptions struct {
 Domains []string `json:"domains"`
 ScanLevel string `json:"scanLevel"`
 Repeats int `json:"repeats"`
 CheckQUIC bool `json:"checkQuic"`
 CheckIPv6 bool `json:"checkIpv6"`
}

type DiagnosticResult struct {
 BestProfile string `json:"bestProfile,omitempty"`
 Summary string `json:"summary"`
 Domains []string `json:"domains"`
 DurationSeconds float64 `json:"durationSeconds"`
 Calibration *Calibration `json:"calibration,omitempty"`
 Warnings []string `json:"warnings,omitempty"`
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

func RunBlockcheck(ctx context.Context,options DiagnosticOptions) (result DiagnosticResult,resultErr error) {
 domains:=append([]string(nil),options.Domains...)
 level:=options.ScanLevel
 if level==""{level="standard"}
 if level!="quick" && level!="standard" && level!="force"{return DiagnosticResult{},fmt.Errorf("invalid blockcheck scan level %q",level)}
 repeats:=options.Repeats
 if repeats==0{repeats=2}
 if repeats<1||repeats>3{return DiagnosticResult{},fmt.Errorf("repeats must be between 1 and 3")}
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
 deadline:=15*time.Minute
 if level=="force"{deadline=30*time.Minute}
 ctx,cancel:=context.WithTimeout(ctx,deadline)
 defer cancel()
 // Official blockcheck2 needs anti-DPI handlers disabled during tests.
 // Preserve active VPN traffic by removing only the marked zapret2 rules:
 // normal outbounds and other firewall tables are unchanged.
 resume,err:=SuspendForDiagnostics(ctx)
 if err!=nil{return DiagnosticResult{},err}
 defer func(){
  recoverCtx,recoverCancel:=context.WithTimeout(context.Background(),20*time.Second)
  defer recoverCancel()
  resultErr=errors.Join(resultErr,resume(recoverCtx))
 }()
 cmd:=exec.CommandContext(ctx,"/bin/sh",script)
 // Send SIGTERM rather than the default SIGKILL on cancellation so that
 // upstream trap handlers can remove temporary test nftables rules.
 cmd.Cancel=func()error{
  if cmd.Process==nil{return os.ErrProcessDone}
  return cmd.Process.Signal(syscall.SIGTERM)
 }
 cmd.WaitDelay=5*time.Second
 cmd.Dir=Directory()
 cmd.Env=append(os.Environ(),
  "BATCH=1",
  "TEST=standard",
  "DOMAINS="+strings.Join(domains," "),
  "SCANLEVEL="+level,
  fmt.Sprintf("REPEATS=%d",repeats),
  fmt.Sprintf("ENABLE_HTTP3=%d",boolAsInt(options.CheckQUIC)),
  "IPVS="+diagnosticIPVersions(options.CheckIPv6),
  "PARALLEL=0",
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
 // Preserve the full captured SUMMARY for ranking. The response can be
 // truncated for the UI, but truncation before parsing could remove the
 // header or highest-coverage strategies from a large scan.
 fullSummary:=full
 if pos:=strings.LastIndex(full,"* SUMMARY");pos>=0{fullSummary=full[pos:]}
 visibleSummary:=fullSummary
 if len(visibleSummary)>14000{visibleSummary=visibleSummary[len(visibleSummary)-14000:]}
 result=DiagnosticResult{Summary:visibleSummary,Domains:domains,DurationSeconds:time.Since(start).Seconds()}
 // The upstream SUMMARY and COVERAGE sections contain successful exact
 // options. Do not infer a result from intermediate "working" log lines.
 calibrated,parseErr:=ParseCalibrationSummary(fullSummary,domains,level)
 if parseErr!=nil {
  result.Warnings=append(result.Warnings,parseErr.Error())
 } else {
  // Validate all selected arguments against this exact installed binary.
  // The dry-run checks the complete multi-profile command and avoids
  // applying an invalid recommendation to the running core.
  opts:=Options{TCP:true,UDP:options.CheckQUIC,Profile:"auto",Calibration:calibrated}
  p:=Plan{Tag:"diagnostic",Options:opts,Queue:2400}
  args,argErr:=strategyArgs(p)
  if argErr==nil{
   check:=exec.CommandContext(ctx,BinaryPath(),append([]string{"--dry-run"},args...)...)
   check.Dir=Directory()
   if message,checkErr:=check.CombinedOutput();checkErr!=nil{
    argErr=fmt.Errorf("official nfqws2 rejected calibrated settings: %w: %s",checkErr,strings.TrimSpace(string(message)))
   }
  }
  if argErr!=nil {result.Warnings=append(result.Warnings,argErr.Error())} else {
   result.Calibration=calibrated
   result.BestProfile="auto"
  }
 }
 if err!=nil{
  if ctx.Err()!=nil{return result,fmt.Errorf("zapret2 blockcheck cancelled or timed out: %w",ctx.Err())}
  return result,fmt.Errorf("official blockcheck2 failed: %w",err)
 }
 return result,nil
}

func diagnosticIPVersions(dual bool)string{
 if dual{return "46"}
 return "4"
}
func boolAsInt(v bool)int{if v{return 1};return 0}

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
