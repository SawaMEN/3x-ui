package oxidns

import (
 "archive/tar"
 "bytes"
 "compress/gzip"
 "context"
 "crypto/sha256"
 "crypto/tls"
 "crypto/x509"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strconv"
 "strings"
 "sync"
 "time"
)

const (
 Version = "v1.6.1"
 StateDir = "/etc/x-ui/oxidns"
 binaryPath = StateDir + "/bin/oxidns"
 configPath = StateDir + "/config.yaml"
 statePath = StateDir + "/settings.json"
 servicePath = "/etc/systemd/system/x-ui-oxidns.service"
 localPort = 15353
)

var mu sync.Mutex

type Settings struct {
 Enabled bool `json:"enabled"`
 Domain string `json:"domain"`
 DoH bool `json:"doh"`
 Port int `json:"port"`
 Upstream string `json:"upstream"`
 UseSingBox bool `json:"useSingBox"`
}

type Status struct {
 Settings
 Installed bool `json:"installed"`
 Running bool `json:"running"`
 DoHURL string `json:"dohUrl,omitempty"`
 LocalAddress string `json:"localAddress"`
 Version string `json:"version"`
 Error string `json:"error,omitempty"`
}

func Defaults() Settings { return Settings{Port:8443, Upstream:"quad9", UseSingBox:true} }

func Load() (Settings,error) {
 c:=Defaults()
 data,err:=os.ReadFile(statePath)
 if errors.Is(err,os.ErrNotExist) {return c,nil}
 if err!=nil{return c,err}
 if err:=json.Unmarshal(data,&c);err!=nil{return c,err}
 return c,nil
}

func CurrentStatus(ctx context.Context) (Status,error) {
 cfg,err:=Load()
 if err!=nil{return Status{},err}
 s:=Status{Settings:cfg,LocalAddress:net.JoinHostPort("127.0.0.1",strconv.Itoa(localPort)),Version:Version}
 if _,err:=os.Stat(binaryPath);err==nil{s.Installed=true}
 if cfg.DoH && cfg.Domain!="" {
  port:=""
  if cfg.Port!=443 {port=":"+strconv.Itoa(cfg.Port)}
  s.DoHURL="https://"+cfg.Domain+port+"/dns-query"
 }
 if s.Installed {
  cmd:=exec.CommandContext(ctx,"systemctl","is-active","--quiet","x-ui-oxidns.service")
  s.Running=cmd.Run()==nil
 }
 return s,nil
}

func validate(c Settings, certFile,keyFile string) error {
 if c.Upstream!="quad9" && c.Upstream!="cloudflare" && c.Upstream!="adguard" {
  return errors.New("выберите DNS-провайдера")
 }
 if c.Port<1||c.Port>65535||c.Port==localPort {return errors.New("неверный HTTPS-порт")}
 if c.DoH {
  if len(c.Domain)<4||len(c.Domain)>253||strings.ContainsAny(c.Domain," /:\	
")||strings.HasSuffix(c.Domain,".") {
   return errors.New("укажите корректное доменное имя DoH")
  }
  if net.ParseIP(c.Domain)!=nil||!strings.Contains(c.Domain,".") {return errors.New("для DoH необходимо доменное имя")}
  if certFile==""||keyFile=="" {return errors.New("сначала настройте TLS-сертификат панели")}
  pair,err:=tls.LoadX509KeyPair(certFile,keyFile)
  if err!=nil{return fmt.Errorf("не удалось прочитать TLS-сертификат: %w",err)}
  leaf,err:=x509.ParseCertificate(pair.Certificate[0])
  if err!=nil{return err}
  if err:=leaf.VerifyHostname(c.Domain);err!=nil{return fmt.Errorf("сертификат не подходит домену: %w",err)}
  if time.Now().After(leaf.NotAfter)||time.Now().Before(leaf.NotBefore) {
   return errors.New("TLS-сертификат просрочен или ещё не действует")
  }
 }
 return nil
}

func upstream(c Settings) (string,string) {
 switch c.Upstream {
 case "cloudflare": return "tls://one.one.one.one:853","1.1.1.1:53"
 case "adguard": return "tls://dns.adguard-dns.com:853","94.140.14.14:53"
 default: return "tls://dns.quad9.net:853","9.9.9.9:53"
 }
}

// JSON-quoted scalar strings are valid YAML strings and cannot inject plugin fields.
func Generate(c Settings, certFile,keyFile string) (string,error) {
 if err:=validate(c,certFile,keyFile);err!=nil{return "",err}
 addr,bootstrap:=upstream(c)
 q:=strconv.Quote
 var b strings.Builder
 b.WriteString("log:
  level: info
plugins:
")
 b.WriteString("  - tag: forward_upstream
    type: forward
    args:
      upstreams:
")
 b.WriteString("        - addr: "+q(addr)+"
          bootstrap: "+q(bootstrap)+"
")
 b.WriteString("  - tag: main
    type: sequence
    args:
      - exec: "$forward_upstream"
      - exec: accept
")
 for _,scheme:=range []string{"udp","tcp"} {
  b.WriteString("  - tag: "+scheme+"_local
    type: "+scheme+"_server
    args:
      entry: main
      listen: "+q(net.JoinHostPort("127.0.0.1",strconv.Itoa(localPort)))+"
")
 }
 if c.DoH {
  b.WriteString("  - tag: doh_public
    type: http_server
    args:
")
  b.WriteString("      listen: "+q(":"+strconv.Itoa(c.Port))+"
")
  b.WriteString("      cert: "+q(certFile)+"
      key: "+q(keyFile)+"
")
  b.WriteString("      entries:
        - path: "/dns-query"
          exec: main
")
 }
 return b.String(),nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
 if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{return err}
 f,err:=os.CreateTemp(filepath.Dir(path),".oxidns-*")
 if err!=nil{return err}
 defer os.Remove(f.Name())
 if err:=f.Chmod(mode);err!=nil{f.Close();return err}
 if _,err:=f.Write(data);err!=nil{f.Close();return err}
 if err:=f.Sync();err!=nil{f.Close();return err}
 if err:=f.Close();err!=nil{return err}
 return os.Rename(f.Name(),path)
}

func systemctl(ctx context.Context, args ...string) error {
 cmd:=exec.CommandContext(ctx,"systemctl",args...)
 out,err:=cmd.CombinedOutput()
 if err!=nil{return fmt.Errorf("systemctl %s: %w (%s)",strings.Join(args," "),err,strings.TrimSpace(string(out)))}
 return nil
}

const unit = "[Unit]
Description=3x-ui OxiDNS
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
ExecStart="+binaryPath+" start -c "+configPath+" -d "+StateDir+"/data
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
[Install]
WantedBy=multi-user.target
"

func asset() (string,string,error) {
 switch runtime.GOOS+"/"+runtime.GOARCH {
 case "linux/amd64":return "oxidns-x86_64-unknown-linux-musl.tar.gz","2ac8e103e03407a73c29da848f6ce2d524e751ce1da8a744aa9168fc1384e3b0",nil
 case "linux/arm64":return "oxidns-aarch64-unknown-linux-musl.tar.gz","d879d4b9199c5c911d296af4df8d9e974523fb98a6ee2418ae7274d152365e42",nil
 default:return "","",fmt.Errorf("автоматическая установка поддерживает только Linux amd64/arm64, текущая система: %s/%s",runtime.GOOS,runtime.GOARCH)
 }
}

func install(ctx context.Context) error {
 if _,err:=exec.LookPath("systemctl");err!=nil{return errors.New("для автоматического запуска требуется systemd (Docker пока не поддерживается)")}
 name,digest,err:=asset()
 if err!=nil{return err}
 if err:=os.MkdirAll(StateDir+"/bin",0700);err!=nil{return err}
 if _,err:=os.Stat(binaryPath);errors.Is(err,os.ErrNotExist) {
  url:="https://github.com/svenshi/oxidns/releases/download/"+Version+"/"+name
  req,err:=http.NewRequestWithContext(ctx,http.MethodGet,url,nil)
  if err!=nil{return err}
  client:=&http.Client{Timeout:120*time.Second}
  res,err:=client.Do(req)
  if err!=nil{return err}
  defer res.Body.Close()
  if res.StatusCode!=200{return fmt.Errorf("GitHub вернул HTTP %d",res.StatusCode)}
  data,err:=io.ReadAll(io.LimitReader(res.Body,32<<20))
  if err!=nil{return err}
  if len(data)>=32<<20{return errors.New("архив превышает допустимый размер")}
  got:=sha256.Sum256(data)
  if hex.EncodeToString(got[:])!=digest{return errors.New("контрольная сумма официального OxiDNS не совпадает")}
  gz,err:=gzip.NewReader(bytes.NewReader(data))
  if err!=nil{return err}
  defer gz.Close()
  archive:=tar.NewReader(gz)
  found:=false
  for {
   hdr,err:=archive.Next()
   if errors.Is(err,io.EOF){break}
   if err!=nil{return err}
   if hdr.Typeflag!=tar.TypeReg && hdr.Typeflag!=tar.TypeRegA {continue}
   if filepath.Base(hdr.Name)!="oxidns" {continue}
   if hdr.Size<=0||hdr.Size>32<<20{return errors.New("неверный размер исполняемого файла")}
   executable,err:=io.ReadAll(io.LimitReader(archive,hdr.Size))
   if err!=nil{return err}
   if int64(len(executable))!=hdr.Size{return errors.New("неполный исполняемый файл")}
   if err:=atomicWrite(binaryPath,executable,0755);err!=nil{return err}
   found=true
   break
  }
  if !found{return errors.New("в официальном архиве не найден бинарник oxidns")}
 } else if err!=nil{return err}
 if err:=atomicWrite(servicePath,[]byte(unit),0644);err!=nil{return err}
 return systemctl(ctx,"daemon-reload")
}

func Setup(ctx context.Context, c Settings, certFile,keyFile string) error {
 mu.Lock()
 defer mu.Unlock()
 c.Enabled=true
 if err:=validate(c,certFile,keyFile);err!=nil{return err}
 yaml,err:=Generate(c,certFile,keyFile)
 if err!=nil{return err}
 if err:=install(ctx);err!=nil{return err}
 oldYAML,_:=os.ReadFile(configPath)
 oldState,_:=os.ReadFile(statePath)
 if err:=atomicWrite(configPath,[]byte(yaml),0600);err!=nil{return err}
 rollback:=func() {
  if len(oldYAML)>0 {_=atomicWrite(configPath,oldYAML,0600)} else {_=os.Remove(configPath)}
  if len(oldState)>0 {_=atomicWrite(statePath,oldState,0600)} else {_=os.Remove(statePath)}
  if len(oldYAML)>0 {_=systemctl(ctx,"restart","x-ui-oxidns.service")} else {_=systemctl(ctx,"stop","x-ui-oxidns.service")}
 }
 if out,err:=exec.CommandContext(ctx,binaryPath,"check","-c",configPath).CombinedOutput();err!=nil {
  rollback()
  return fmt.Errorf("OxiDNS отклонил конфигурацию: %w (%s)",err,strings.TrimSpace(string(out)))
 }
 if err:=systemctl(ctx,"enable","x-ui-oxidns.service");err!=nil{rollback();return err}
 if err:=systemctl(ctx,"restart","x-ui-oxidns.service");err!=nil{rollback();return err}
 data,err:=json.Marshal(c)
 if err!=nil{rollback();return err}
 if err:=atomicWrite(statePath,data,0600);err!=nil{rollback();return err}
 return nil
}

func Disable(ctx context.Context) error {
 mu.Lock()
 defer mu.Unlock()
 c,err:=Load()
 if err!=nil{return err}
 if !c.Enabled{return nil}
 c.Enabled=false
 data,_:=json.Marshal(c)
 if err:=atomicWrite(statePath,data,0600);err!=nil{return err}
 if err:=systemctl(ctx,"stop","x-ui-oxidns.service");err!=nil{return err}
 return systemctl(ctx,"disable","x-ui-oxidns.service")
}

const Tag = "oxidns-local"

// WithSingBoxDNS leaves user DNS rules and upstreams intact, changing only the default.
func WithSingBoxDNS(src map[string]any) (map[string]any,error) {
 next:=make(map[string]any,len(src)+2)
 for k,v:=range src{next[k]=v}
 servers:=make([]map[string]any,0)
 switch rows:=src["servers"].(type) {
 case []map[string]any: servers=append(servers,rows...)
 case []any:
  for _,v:=range rows {
   m,ok:=v.(map[string]any);if !ok{return nil,errors.New("неверный DNS-сервер sing-box")}
   servers=append(servers,m)
  }
 case nil:
 default:return nil,errors.New("неверный список DNS-серверов sing-box")
 }
 for _,row:=range servers {
  if row["tag"]==Tag {return nil,fmt.Errorf("DNS-тег %q уже занят",Tag)}
 }
 servers=append(servers,map[string]any{"type":"udp","tag":Tag,"server":"127.0.0.1","server_port":localPort})
 next["servers"]=servers
 next["final"]=Tag
 return next,nil
}
