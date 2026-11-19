package zapret2

import (
 "bytes"
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
  if !diagnosticsHost.MatchString(domain)||!strings.Contains(domain,".")||
   strings.EqualFold(domain,"localhost")||strings.HasSuffix(strings.ToLower(domain),".local")||
   strings.HasSuffix(strings.ToLower(domain),".internal") {
    return fmt.Errorf("invalid public test domain %q",domain)
  }
 }
 return nil
}

func RunBlockcheck(ctx context.Context,domains []string) (DiagnosticResult,error) {
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
  lowered:=strings.ToLower(summary)
  switch {
  case strings.Contains(lowered,"tls_mod=rnd")||strings.Contains(lowered,"rndsni"):
   result.BestProfile="aggressive"
  case strings.Contains(lowered,"multidisorder")||strings.Contains(lowered,"tcp_md5"):
   result.BestProfile="balanced"
  case strings.Contains(lowered,"multisplit"):
   result.BestProfile="safe"
  }
 }
 if err!=nil{
  if ctx.Err()!=nil{return result,fmt.Errorf("zapret2 blockcheck cancelled or timed out: %w",ctx.Err())}
  return result,fmt.Errorf("official blockcheck2 failed: %w",err)
 }
 return result,nil
}

type cappedLog struct{bytes.Buffer}
func (w *cappedLog) Write(p []byte)(int,error){
 const capBytes=1<<20
 n:=len(p)
 if w.Len()<capBytes {
  remain:=capBytes-w.Len()
  if remain>len(p){remain=len(p)}
  _,_=w.Buffer.Write(p[:remain])
 }
 return n,nil
}
