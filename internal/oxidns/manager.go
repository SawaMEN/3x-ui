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
 InstalledVersion string `json:"installedVersion"`
 LatestVersion string `json:"latestVersion"`
 UpdateAvailable bool `json:"updateAvailable"`
 Supported bool `json:"supported"`
 UnsupportedReason string `json:"unsupportedReason,omitempty"`
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
 s.InstalledVersion=installedVersion(ctx)
 _,err=releaseAssetName()
 s.Supported=err==nil
 if err!=nil{s.UnsupportedReason=err.Error()}
 if s.Supported && os.Geteuid()!=0{s.Supported=false;s.UnsupportedReason="для установки требуется запуск панели от root"}
 if s.Supported {if _,systemErr:=exec.LookPath("systemctl");systemErr!=nil{s.Supported=false;s.UnsupportedReason="не найден systemd (Docker не поддерживается)"}}
 if s.InstalledVersion!=""{s.Version=s.InstalledVersion}
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
 if _,_,valid:=upstream(c);!valid {return errors.New("выберите DNS-провайдера")}
 if c.Port<1 || c.Port>65535 || c.Port==localPort {return errors.New("неверный HTTPS-порт")}
 if c.DoH {
  if len(c.Domain)<4 || len(c.Domain)>253 || !strings.Contains(c.Domain,".") || net.ParseIP(c.Domain)!=nil {return errors.New("укажите доменное имя для DoH")}
  for _,ch:=range c.Domain {
   if !((ch>='a' && ch<='z')||(ch>='A' && ch<='Z')||(ch>='0' && ch<='9')||ch=='.'||ch=='-') {return errors.New("недопустимые символы в домене DoH")}
  }
  if certFile==""||keyFile=="" {return errors.New("сначала настройте TLS-сертификат панели")}
  pair,err:=tls.LoadX509KeyPair(certFile,keyFile)
  if err!=nil{return fmt.Errorf("не удалось прочитать TLS-сертификат: %w",err)}
  leaf,err:=x509.ParseCertificate(pair.Certificate[0])
  if err!=nil{return err}
  if err:=leaf.VerifyHostname(c.Domain);err!=nil{return fmt.Errorf("сертификат не подходит домену: %w",err)}
  if time.Now().After(leaf.NotAfter)||time.Now().Before(leaf.NotBefore){return errors.New("TLS-сертификат просрочен или ещё не действует")}
 }
 return nil
}

// DNS upstreams use authenticated TLS and official bootstrap addresses.
func upstream(c Settings) (address,bootstrap string,valid bool) {
 switch c.Upstream {
 case "quad9": return "tls://dns.quad9.net:853","9.9.9.9:53",true
 case "cloudflare": return "tls://one.one.one.one:853","1.1.1.1:53",true
 case "adguard": return "tls://dns.adguard-dns.com:853","94.140.14.14:53",true
 case "google": return "tls://dns.google:853","8.8.8.8:53",true
 case "yandex-basic": return "tls://common.dot.dns.yandex.net:853","77.88.8.8:53",true
 case "yandex-safe": return "tls://safe.dot.dns.yandex.net:853","77.88.8.88:53",true
 case "yandex-family": return "tls://family.dot.dns.yandex.net:853","77.88.8.7:53",true
 default: return "","",false
 }
}

// JSON-quoted scalar strings are valid YAML strings and cannot inject plugin fields.
func Generate(c Settings, certFile,keyFile string) (string,error) {
 if err:=validate(c,certFile,keyFile);err!=nil{return "",err}
 addr,bootstrap,_:=upstream(c)
 var b strings.Builder
 lines:=[]string{"log:","  level: info","plugins:","  - tag: forward_upstream","    type: forward","    args:","      upstreams:"}
 for _,line:=range lines{fmt.Fprintln(&b,line)}
 fmt.Fprintln(&b,"        - addr: "+strconv.Quote(addr))
 fmt.Fprintln(&b,"          bootstrap: "+strconv.Quote(bootstrap))
 for _,line:=range []string{"  - tag: main","    type: sequence","    args:",`      - exec: "$forward_upstream"`,"      - exec: accept"}{fmt.Fprintln(&b,line)}
 for _,scheme:=range []string{"udp","tcp"} {
  fmt.Fprintln(&b,"  - tag: "+scheme+"_local")
  fmt.Fprintln(&b,"    type: "+scheme+"_server")
  fmt.Fprintln(&b,"    args:")
  fmt.Fprintln(&b,"      entry: main")
  fmt.Fprintln(&b,"      listen: "+strconv.Quote(net.JoinHostPort("127.0.0.1",strconv.Itoa(localPort))))
 }
 if c.DoH {
  fmt.Fprintln(&b,"  - tag: doh_public")
  fmt.Fprintln(&b,"    type: http_server")
  fmt.Fprintln(&b,"    args:")
  fmt.Fprintln(&b,"      listen: "+strconv.Quote(":"+strconv.Itoa(c.Port)))
  fmt.Fprintln(&b,"      cert: "+strconv.Quote(certFile))
  fmt.Fprintln(&b,"      key: "+strconv.Quote(keyFile))
  fmt.Fprintln(&b,"      entries:")
  fmt.Fprintln(&b,`        - path: "/dns-query"`)
  fmt.Fprintln(&b,"          exec: main")
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

var unit = strings.Join([]string{"[Unit]","Description=3x-ui OxiDNS","After=network-online.target","Wants=network-online.target","[Service]","Type=simple","ExecStart=/etc/x-ui/oxidns/bin/oxidns start -c /etc/x-ui/oxidns/config.yaml -d /etc/x-ui/oxidns/data","Restart=on-failure","RestartSec=3","NoNewPrivileges=true","[Install]","WantedBy=multi-user.target"},string([]byte{10}))+string([]byte{10})

type releaseAsset struct {
 Name string `json:"name"`
 Digest string `json:"digest"`
 URL string `json:"browser_download_url"`
}

type upstreamRelease struct {
 Tag string `json:"tag_name"`
 Assets []releaseAsset `json:"assets"`
}

func releaseAssetName() (string,error) {
 switch runtime.GOOS+"/"+runtime.GOARCH {
 case "linux/amd64":return "oxidns-x86_64-unknown-linux-musl.tar.gz",nil
 case "linux/arm64":return "oxidns-aarch64-unknown-linux-musl.tar.gz",nil
 default:return "",fmt.Errorf("OxiDNS поддерживается только на Linux amd64 и arm64 (%s/%s)",runtime.GOOS,runtime.GOARCH)
 }
}

func Latest(ctx context.Context) (string,*releaseAsset,error) {
 name,err:=releaseAssetName()
 if err!=nil{return "",nil,err}
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,"https://api.github.com/repos/svenshi/oxidns/releases/latest",nil)
 if err!=nil{return "",nil,err}
 req.Header.Set("Accept","application/vnd.github+json")
 client:=&http.Client{Timeout:20*time.Second}
 resp,err:=client.Do(req)
 if err!=nil{return "",nil,err}
 defer resp.Body.Close()
 if resp.StatusCode!=200{return "",nil,fmt.Errorf("GitHub API: HTTP %d",resp.StatusCode)}
 var rel upstreamRelease
 if err:=json.NewDecoder(io.LimitReader(resp.Body,1<<20)).Decode(&rel);err!=nil{return "",nil,err}
 if len(rel.Tag)<6 || !strings.HasPrefix(rel.Tag,"v") || len(rel.Tag)>64 || strings.ContainsAny(rel.Tag,"/ ?#") {
  return "",nil,errors.New("неверная версия OxiDNS из GitHub")
 }
 for _,asset:=range rel.Assets{
  expectedURL:="https://github.com/svenshi/oxidns/releases/download/"+rel.Tag+"/"+name
  if asset.Name==name && asset.URL==expectedURL && strings.HasPrefix(asset.Digest,"sha256:") && len(asset.Digest)==71 {
   if _,err:=hex.DecodeString(strings.TrimPrefix(asset.Digest,"sha256:"));err==nil{
    return rel.Tag,&asset,nil
   }
  }
 }
 return "",nil,errors.New("официальный архив OxiDNS или SHA-256 не найдены")
}

func installedVersion(ctx context.Context) string {
 if _,err:=os.Stat(binaryPath);err!=nil{return ""}
 ctx,cancel:=context.WithTimeout(ctx,3*time.Second)
 defer cancel()
 data,err:=exec.CommandContext(ctx,binaryPath,"--version").Output()
 if err!=nil{return ""}
 for _,field:=range strings.Fields(string(data)){
  part:=strings.TrimPrefix(field,"v")
  if len(part)>0 && part[0]>='0' && part[0]<='9' && strings.Count(part,".")>=2 {
   return "v"+part
  }
 }
 return ""
}

func StatusWithUpdates(ctx context.Context) (Status,error) {
 s,err:=CurrentStatus(ctx)
 if err!=nil{return s,err}
 latest,_,err:=Latest(ctx)
 if err!=nil {s.Error="Не удалось проверить GitHub: "+err.Error();return s,nil}
 s.LatestVersion=latest
 s.UpdateAvailable=s.Installed && s.InstalledVersion!="" && s.InstalledVersion!=latest
 return s,nil
}

func installRelease(ctx context.Context, asset releaseAsset) ([]byte,error) {
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,asset.URL,nil)
 if err!=nil{return nil,err}
 client:=&http.Client{Timeout:120*time.Second}
 res,err:=client.Do(req)
 if err!=nil{return nil,err}
 defer res.Body.Close()
 if res.StatusCode!=200{return nil,fmt.Errorf("GitHub вернул HTTP %d",res.StatusCode)}
 data,err:=io.ReadAll(io.LimitReader(res.Body,32<<20))
 if err!=nil{return nil,err}
 if len(data)>=32<<20{return nil,errors.New("архив превышает ограничение 32 MiB")}
 got:=sha256.Sum256(data)
 if hex.EncodeToString(got[:])!=strings.TrimPrefix(asset.Digest,"sha256:") {
  return nil,errors.New("контрольная сумма официального архива не совпадает")
 }
 gz,err:=gzip.NewReader(bytes.NewReader(data))
 if err!=nil{return nil,err}
 defer gz.Close()
 archive:=tar.NewReader(gz)
 for{
  hdr,err:=archive.Next()
  if errors.Is(err,io.EOF){break}
  if err!=nil{return nil,err}
  if hdr.Typeflag!=tar.TypeReg && hdr.Typeflag!=tar.TypeRegA {continue}
  if filepath.Base(hdr.Name)!="oxidns"{continue}
  if hdr.Size<1 || hdr.Size>32<<20{return nil,errors.New("неверный размер бинарника")}
  data,err:=io.ReadAll(io.LimitReader(archive,hdr.Size))
  if err!=nil{return nil,err}
  if int64(len(data))!=hdr.Size{return nil,errors.New("неполный бинарник OxiDNS")}
  return data,nil
 }
 return nil,errors.New("не найден исполняемый файл oxidns")
}

func installOnly(ctx context.Context) error {
 if _,err:=exec.LookPath("systemctl");err!=nil{return errors.New("для OxiDNS требуется Linux с systemd")}
 if _,err:=releaseAssetName();err!=nil{return err}
 if _,err:=os.Stat(binaryPath);errors.Is(err,os.ErrNotExist) {
  _,asset,err:=Latest(ctx)
  if err!=nil{return err}
  executable,err:=installRelease(ctx,*asset)
  if err!=nil{return err}
  if err:=atomicWrite(binaryPath,executable,0755);err!=nil{return err}
 } else if err!=nil{return err}
 if err:=atomicWrite(servicePath,[]byte(unit),0644);err!=nil{return err}
 return systemctl(ctx,"daemon-reload")
}

func InstallOnly(ctx context.Context) error {
 mu.Lock()
 defer mu.Unlock()
 return installOnly(ctx)
}

func waitHealthy(ctx context.Context) error {
 address:=net.JoinHostPort("127.0.0.1",strconv.Itoa(localPort))
 for attempt:=0;attempt<12;attempt++{
  conn,err:=net.DialTimeout("tcp",address,300*time.Millisecond)
  if err==nil{_ =conn.Close();return nil}
  select{
  case <-ctx.Done():return ctx.Err()
  case <-time.After(300*time.Millisecond):
  }
 }
 return errors.New("OxiDNS не открыл локальный DNS-порт после запуска")
}

func Upgrade(ctx context.Context) error {
 mu.Lock()
 defer mu.Unlock()
 if _,err:=os.Stat(binaryPath);err!=nil{return errors.New("сначала установите OxiDNS")}
 latest,asset,err:=Latest(ctx)
 if err!=nil{return err}
 if installedVersion(ctx)==latest{return nil}
 executable,err:=installRelease(ctx,*asset)
 if err!=nil{return err}
 oldBinary,err:=os.ReadFile(binaryPath)
 if err!=nil{return err}
 c,err:=Load()
 if err!=nil{return err}
 if err:=atomicWrite(binaryPath,executable,0755);err!=nil{return err}
 if c.Enabled{
  restartErr:=systemctl(ctx,"restart","x-ui-oxidns.service")
  if restartErr==nil{restartErr=waitHealthy(ctx)}
  if restartErr!=nil{
   err:=restartErr
   restoreErr:=atomicWrite(binaryPath,oldBinary,0755)
   if restoreErr==nil {restoreErr=systemctl(ctx,"restart","x-ui-oxidns.service")}
   return errors.Join(err,restoreErr)
  }
 }
 return nil
}

func Uninstall(ctx context.Context) error {
 mu.Lock()
 defer mu.Unlock()
 cfg,err:=Load()
 if err!=nil{return err}
 cfg.Enabled=false
 data,_:=json.Marshal(cfg)
 if err:=atomicWrite(statePath,data,0600);err!=nil{return err}
 if err:=systemctl(ctx,"stop","x-ui-oxidns.service");err!=nil{
  if !strings.Contains(err.Error(),"not loaded") && !strings.Contains(err.Error(),"not found"){return err}
 }
 if err:=systemctl(ctx,"disable","x-ui-oxidns.service");err!=nil{
  if !strings.Contains(err.Error(),"does not exist") && !strings.Contains(err.Error(),"not loaded"){return err}
 }
 for _,path:=range []string{binaryPath,servicePath,configPath}{
  if err:=os.Remove(path);err!=nil&&!errors.Is(err,os.ErrNotExist){return err}
 }
 return systemctl(ctx,"daemon-reload")
}

func Setup(ctx context.Context, c Settings, certFile,keyFile string) error {
 mu.Lock()
 defer mu.Unlock()
 c.Enabled=true
 if err:=validate(c,certFile,keyFile);err!=nil{return err}
 yaml,err:=Generate(c,certFile,keyFile)
 if err!=nil{return err}
 if err:=installOnly(ctx);err!=nil{return err}
 oldYAML,_:=os.ReadFile(configPath)
 oldState,_:=os.ReadFile(statePath)
 if err:=atomicWrite(configPath,[]byte(yaml),0600);err!=nil{return err}
 rollback:=func() {
  if len(oldYAML)>0 {_=atomicWrite(configPath,oldYAML,0600)} else {_=os.Remove(configPath)}
  if len(oldState)>0 {_=atomicWrite(statePath,oldState,0600)} else {_=os.Remove(statePath)}
  if len(oldYAML)>0 {_=systemctl(ctx,"restart","x-ui-oxidns.service")} else {_=systemctl(ctx,"stop","x-ui-oxidns.service");_=systemctl(ctx,"disable","x-ui-oxidns.service")}
 }
 if out,err:=exec.CommandContext(ctx,binaryPath,"check","-c",configPath).CombinedOutput();err!=nil {
  rollback()
  return fmt.Errorf("OxiDNS отклонил конфигурацию: %w (%s)",err,strings.TrimSpace(string(out)))
 }
 if err:=systemctl(ctx,"enable","x-ui-oxidns.service");err!=nil{rollback();return err}
 if err:=systemctl(ctx,"restart","x-ui-oxidns.service");err!=nil{rollback();return err}
 if err:=waitHealthy(ctx);err!=nil{rollback();return err}
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
