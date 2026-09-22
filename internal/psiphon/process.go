package psiphon

import (
  "context"
  "errors"
  "fmt"
  "os"
  "os/exec"
  "path/filepath"
  "runtime"
  "strings"
  "sync"
  "sync/atomic"
  "syscall"
  "time"

  "github.com/SawaMEN/3x-ui/v3/internal/config"
  "github.com/SawaMEN/3x-ui/v3/internal/logger"
)

func GetBinaryName()string{n:=fmt.Sprintf("psiphond-%s-%s",runtime.GOOS,runtime.GOARCH);if runtime.GOOS=="windows"{n+=".exe"};return n}
func GetBinaryPath()string{custom:=filepath.Join(config.GetBinFolderPath(),GetBinaryName());if _,err:=os.Stat(custom);err==nil{return custom};short:=filepath.Join(config.GetBinFolderPath(),"psiphond");if runtime.GOOS=="windows"{short+=".exe"};if _,err:=os.Stat(short);err==nil{return short};for _,p:=range []string{"/usr/local/bin/psiphond","/usr/bin/psiphond"}{if _,err:=os.Stat(p);err==nil{return p}};if p,err:=exec.LookPath("psiphond");err==nil{return p};return short}

type Process struct{mu sync.RWMutex;cmd *exec.Cmd;done chan struct{};dir,label string;inst Instance;exitErr error;intentionalStop atomic.Bool}
func newProcess(dir,label string,inst Instance)*Process{return &Process{dir:dir,label:label,inst:inst}}
func(p *Process)IsRunning()bool{p.mu.RLock();cmd,done:=p.cmd,p.done;p.mu.RUnlock();if cmd==nil||cmd.Process==nil{return false};if done!=nil{select{case<-done:return false;default:}};return true}
func(p *Process)Start()error{
  if p.IsRunning(){return errors.New("psiphond is already running")}
  entry:=filepath.Join(p.dir,"server-entry.dat");if _,err:=os.Stat(entry);os.IsNotExist(err){
    args:=[]string{"generate","-ipaddress",p.inst.ServerAddress,"-protocol",fmt.Sprintf("%s:%d",p.inst.Protocol,p.inst.Port)};args=append(args,p.inst.AdditionalArguments...)
    gen:=exec.Command(GetBinaryPath(),args...);gen.Dir=p.dir;gen.Stdout=&logWriter{label:p.label+" generate"};gen.Stderr=gen.Stdout
    if err:=gen.Run();err!=nil{return fmt.Errorf("psiphon generate failed: %w",err)}
  }
  cmd:=exec.CommandContext(context.Background(),GetBinaryPath(),"run");cmd.Dir=p.dir;lw:=&logWriter{label:p.label};cmd.Stdout=lw;cmd.Stderr=lw
  done:=make(chan struct{});p.mu.Lock();p.cmd,p.done,p.exitErr=cmd,done,nil;p.mu.Unlock();p.intentionalStop.Store(false)
  if err:=cmd.Start();err!=nil{close(done);p.mu.Lock();p.cmd=nil;p.mu.Unlock();return err};go p.wait(cmd,done);return nil
}
func(p *Process)ServerEntry()string{b,err:=os.ReadFile(filepath.Join(p.dir,"server-entry.dat"));if err!=nil{return ""};return strings.TrimSpace(string(b))}
func(p *Process)wait(cmd *exec.Cmd,done chan struct{}){defer close(done);err:=cmd.Wait();if err==nil||p.intentionalStop.Load(){return};logger.Errorf("psiphon: psiphond %s exited: %v",p.label,err);p.mu.Lock();p.exitErr=err;p.mu.Unlock()}
func(p *Process)Stop()error{if !p.IsRunning(){return errors.New("psiphond is not running")};p.intentionalStop.Store(true);p.mu.RLock();cmd,done:=p.cmd,p.done;p.mu.RUnlock();if cmd==nil||cmd.Process==nil{return errors.New("psiphond is not running")};if runtime.GOOS=="windows"{_=cmd.Process.Kill();return waitForExit(done,2*time.Second)};if err:=cmd.Process.Signal(syscall.SIGTERM);err!=nil&& !errors.Is(err,os.ErrProcessDone){return err};if err:=waitForExit(done,5*time.Second);err==nil{return nil};_=cmd.Process.Kill();return waitForExit(done,2*time.Second)}
func waitForExit(done <-chan struct{},timeout time.Duration)error{timer:=time.NewTimer(timeout);defer timer.Stop();select{case<-done:return nil;case<-timer.C:return fmt.Errorf("timed out waiting for psiphond after %s",timeout)}}
type logWriter struct{mu sync.Mutex;label,buf string}
func(w *logWriter)Write(p []byte)(int,error){w.mu.Lock();defer w.mu.Unlock();w.buf+=string(p);for{idx:=strings.IndexByte(w.buf,'\n');if idx<0{break};line:=strings.TrimSpace(w.buf[:idx]);w.buf=w.buf[idx+1:];if line!=""{logger.Infof("psiphon: psiphond %s | %s",w.label,line)}};return len(p),nil}