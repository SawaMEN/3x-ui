package zapret2

// Dedicated NFQUEUE runtime: only traffic emitted by the generated marked
// direct outbounds is observed. No global OUTPUT/PREROUTING interception.
import (
 "bytes"
 "context"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "reflect"
 "strings"
 "sync"
 "syscall"
 "time"
)

const nftTable = "xui_zapret2"

type instance struct {
 cmd *exec.Cmd
 done chan struct{}
 cancel context.CancelFunc
}
var runtimeState struct {
 sync.Mutex
 instances []*instance
 plans []Plan
}

func Running() bool {
 runtimeState.Lock()
 defer runtimeState.Unlock()
 for _,p:=range runtimeState.instances {
  select {case <-p.done: default:return true}
 }
 return false
}

func strategyArgs(p Plan) ([]string,error) {
 opts:=p.Options
 args:=[]string{
  fmt.Sprintf("--qnum=%d",p.Queue),
  "--lua-init=@"+filepath.Join(Directory(),"lua","zapret-lib.lua"),
  "--lua-init=@"+filepath.Join(Directory(),"lua","zapret-antidpi.lua"),
 }
 if opts.TCP {
  switch opts.Profile {
  case "safe":
   args=append(args,"--filter-tcp=80,443","--filter-l7=tls,http","--payload=tls_client_hello,http_req","--lua-desync=multisplit:pos=1")
  case "auto","balanced":
   // The default preset is upstream's conservative example; the auto mode
   // is eligible for blockcheck2 calibration, never silently changes all routes.
   args=append(args,
    "--filter-tcp=443","--filter-l7=tls","--payload=tls_client_hello",
    "--lua-desync=fake:blob=fake_default_tls:tcp_md5:tcp_seq=-10000",
    "--lua-desync=multidisorder:pos=1,midsld",
    "--new","--filter-tcp=80","--filter-l7=http","--payload=http_req",
    "--lua-desync=fake:blob=fake_default_http:tcp_md5",
   )
  case "aggressive":
   args=append(args,
    "--filter-tcp=80,443","--filter-l7=tls,http",
    "--payload=tls_client_hello",
    "--lua-desync=fake:blob=fake_default_tls:tcp_md5:tls_mod=rnd,rndsni,dupsid",
    "--lua-desync=multidisorder:pos=1,midsld",
    "--payload=http_req",
    "--lua-desync=fake:blob=fake_default_http:tcp_md5",
   )
  default:return nil,fmt.Errorf("invalid zapret2 strategy %q",opts.Profile)
  }
 }
 if opts.UDP {
  if opts.TCP {args=append(args,"--new")}
  args=append(args,"--filter-udp=443","--filter-l7=quic","--payload=quic_initial",
   "--lua-desync=fake:blob=fake_default_quic:repeats=6")
 }
 return args,nil
}

func firewallRules(plans []Plan) string {
 var b strings.Builder
 b.WriteString("table inet "+nftTable+" {\n chain output { type filter hook output priority mangle; policy accept;\n")
 for _,p:=range plans {
  if p.Options.TCP {
   fmt.Fprintf(&b," meta mark 0x%x tcp dport { 80, 443 } queue num %d bypass\n",p.Mark,p.Queue)
  }
  if p.Options.UDP {
   fmt.Fprintf(&b," meta mark 0x%x udp dport 443 queue num %d bypass\n",p.Mark,p.Queue)
  }
 }
 b.WriteString(" }\n}\n")
 return b.String()
}

func nft(ctx context.Context,args []string,input string) error {
 cmd:=exec.CommandContext(ctx,"nft",args...)
 if input!="" {cmd.Stdin=strings.NewReader(input)}
 var output bytes.Buffer
 cmd.Stdout=&output;cmd.Stderr=&output
 if err:=cmd.Run();err!=nil{return fmt.Errorf("nft %s: %w: %s",strings.Join(args," "),err,strings.TrimSpace(output.String()))}
 return nil
}

func dropRules(ctx context.Context) error {
 // Table might not exist on first install or after reboot; avoid clearing
 // any unrelated nftables tables or chains.
 test:=exec.CommandContext(ctx,"nft","list","table","inet",nftTable)
 if err:=test.Run();err!=nil {return nil}
 return nft(ctx,[]string{"delete","table","inet",nftTable},"")
}

func installRules(ctx context.Context,plans []Plan) error {
 if err:=dropRules(ctx);err!=nil{return err}
 if len(plans)==0{return nil}
 return nft(ctx,[]string{"-f","-"},firewallRules(plans))
}

func stopInstance(p *instance) {
 if p==nil || p.cmd==nil || p.cmd.Process==nil{return}
 defer p.cancel()
 _=p.cmd.Process.Signal(syscall.SIGTERM)
 select{
 case <-p.done:
 case <-time.After(1500*time.Millisecond):
  _=p.cmd.Process.Kill()
  <-p.done
 }
}

func stopAll(instances []*instance) {
 for _,p:=range instances{stopInstance(p)}
}

func launch(ctx context.Context,plans []Plan) ([]*instance,error) {
 out:=make([]*instance,0,len(plans))
 for _,p:=range plans {
  args,err:=strategyArgs(p)
  if err!=nil {stopAll(out);return nil,err}
  // Test native parameters and referenced Lua files before changing firewall.
  check:=exec.CommandContext(ctx,BinaryPath(),append([]string{"--dry-run"},args...)...)
  check.Dir=Directory()
  if data,err:=check.CombinedOutput();err!=nil {stopAll(out);return nil,fmt.Errorf("invalid zapret2 strategy %q: %w: %s",p.Tag,err,strings.TrimSpace(string(data)))}
  runCtx,cancel:=context.WithCancel(context.Background())
  proc:=exec.CommandContext(runCtx,BinaryPath(),args...)
  proc.Dir=Directory()
  logDir:=filepath.Join(Directory(),"logs")
  if err:=os.MkdirAll(logDir,0700);err!=nil {cancel();stopAll(out);return nil,err}
  log,err:=os.OpenFile(filepath.Join(logDir,fmt.Sprintf("queue-%d.log",p.Queue)),os.O_CREATE|os.O_WRONLY|os.O_APPEND,0600)
  if err!=nil {cancel();stopAll(out);return nil,err}
  proc.Stdout=log;proc.Stderr=log
  if err:=proc.Start();err!=nil {cancel();_=log.Close();stopAll(out);return nil,err}
  _=log.Close()
  instance:=&instance{cmd:proc,done:make(chan struct{}),cancel:cancel}
  go func(){_ = proc.Wait();cancel();close(instance.done)}()
  out=append(out,instance)
  // Fail on immediate exits; no rule should be applied to a dead queue.
  select{
  case <-instance.done:
   stopAll(out);return nil,fmt.Errorf("nfqws2 queue %d exited during startup; inspect %s",p.Queue,logDir)
  case <-time.After(100*time.Millisecond):
  case <-ctx.Done():
   stopAll(out);return nil,ctx.Err()
  }
 }
 return out,nil
}

func replace(ctx context.Context,plans []Plan) error {
 oldPlans:=runtimeState.plans
 oldInstances:=runtimeState.instances
 if len(plans)>0 && reflect.DeepEqual(oldPlans,plans)&&len(oldInstances)==len(plans) {
  alive:=true
  for _,p:=range oldInstances{select{case <-p.done:alive=false;default:}}
  if alive{return nil}
 }
 if len(plans)>0 {
  if !Installed(){return fmt.Errorf("zapret2 is not installed")}
  if os.Geteuid()!=0 {return fmt.Errorf("zapret2 NFQUEUE requires root/CAP_NET_ADMIN")}
  if _,err:=exec.LookPath("nft");err!=nil{return fmt.Errorf("nftables is required by zapret2: %w",err)}
 }
 if err:=dropRules(ctx);err!=nil{return err}
 stopAll(oldInstances)
 runtimeState.instances=nil
 runtimeState.plans=nil
 if len(plans)==0{return nil}
 fresh,err:=launch(ctx,plans)
 if err!=nil {
  // Restore the previous valid strategy rather than leaving an obsolete queue.
  restoreErr:=restore(ctx,oldPlans)
  return errors.Join(err,restoreErr)
 }
 if err:=installRules(ctx,plans);err!=nil {
  stopAll(fresh)
  restoreErr:=restore(ctx,oldPlans)
  return errors.Join(err,restoreErr)
 }
 runtimeState.instances=fresh
 runtimeState.plans=plans
 return nil
}

func restore(ctx context.Context,previous []Plan) error {
 if len(previous)==0{return nil}
 fresh,err:=launch(ctx,previous)
 if err!=nil{return err}
 if err:=installRules(ctx,previous);err!=nil{stopAll(fresh);return err}
 runtimeState.instances=fresh
 runtimeState.plans=previous
 return nil
}

// Apply participates in the core's provisional restart transaction.
// finish(false) restores the last active NFQUEUE plan on a failed restart.
func Apply(ctx context.Context,plans []Plan) (func(bool) error,error) {
 normalized,err:=AssignQueues(plans)
 if err!=nil{return nil,err}
 runtimeState.Lock()
 previous:=append([]Plan(nil),runtimeState.plans...)
 changed:=!reflect.DeepEqual(previous,normalized)
 err=replace(ctx,normalized)
 runtimeState.Unlock()
 if err!=nil{return nil,err}
 return func(commit bool)error {
  if commit||!changed{return nil}
  recovery,cancel:=context.WithTimeout(context.Background(),20*time.Second)
  defer cancel()
  runtimeState.Lock()
  defer runtimeState.Unlock()
  return replace(recovery,previous)
 },nil
}

func StopAll(ctx context.Context) error {
 finish,err:=Apply(ctx,nil)
 if err!=nil{return err}
 return finish(true)
}

// Ensure nft is installed before giving access to a virtual route.
func Preflight(ctx context.Context,plans []Plan) error {
 if len(plans)==0{return nil}
 if !Installed(){return errors.New("install zapret2 from official releases first")}
 if os.Geteuid()!=0{return errors.New("zapret2 requires root privileges")}
 if _,err:=exec.LookPath("nft");err!=nil{return err}
 for _,p:=range plans{
  args,err:=strategyArgs(p);if err!=nil{return err}
  cmd:=exec.CommandContext(ctx,BinaryPath(),append([]string{"--dry-run"},args...)...)
  cmd.Dir=Directory()
  if data,err:=cmd.CombinedOutput();err!=nil{return fmt.Errorf("zapret2 %q: %w: %s",p.Tag,err,data)}
 }
 return nil
}
