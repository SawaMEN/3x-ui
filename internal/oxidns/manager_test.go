package oxidns

import (
 "strings"
 "reflect"
 "testing"
 "os"
 "os/exec"
 "path/filepath"
)

func TestGenerateLocalAndDoH(t *testing.T) {
 cfg:=Defaults()
 yaml,err:=Generate(cfg,"","")
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(yaml, "listen: \"127.0.0.1:15353\"") {t.Fatal("local DNS listener missing")}
 if strings.Contains(yaml,"http_server"){t.Fatal("DoH must stay off without credentials")}
 for _,v:=range []string{"bootstrap: \"9.9.9.9:53\"","type: udp_server","type: tcp_server"} {
  if !strings.Contains(yaml,v){t.Fatalf("missing %s",v)}
 }
}
func TestRejectUntrustedProvider(t *testing.T) {
 cfg:=Defaults()
 cfg.Upstream="foo\n    type: bad"
 if _,err:=Generate(cfg,"","");err==nil{t.Fatal("unvalidated upstream accepted")}
}
func TestPreserveSingBoxDNSRules(t *testing.T) {
 original:=map[string]any{"servers":[]any{map[string]any{"tag":"google","type":"https","server":"dns.google"}},"rules":[]any{map[string]any{"domain_suffix":[]string{"internal"},"server":"google"}},"final":"google","strategy":"ipv4_only"}
 result,err:=WithSingBoxDNS(original)
 if err!=nil{t.Fatal(err)}
 if result["final"]!=Tag{t.Fatal("wrong default")}
 if original["final"]!="google"{t.Fatal("modified input")}
 if !reflect.DeepEqual(result["rules"],original["rules"]){t.Fatal("replaced routing rules")}
 if len(result["servers"].([]map[string]any))!=2{t.Fatal("user server lost")}
 if _,err:=WithSingBoxDNS(result);err==nil{t.Fatal("must reject colliding managed tag")}
}

func TestDNSProviders(t *testing.T) {
 tests:=[]struct{
  name string
  address string
  bootstrap string
 }{
  {"google","tls://dns.google:853","8.8.8.8:53"},
  {"yandex-basic","tls://common.dot.dns.yandex.net:853","77.88.8.8:53"},
  {"yandex-safe","tls://safe.dot.dns.yandex.net:853","77.88.8.88:53"},
  {"yandex-family","tls://family.dot.dns.yandex.net:853","77.88.8.7:53"},
 }
 for _,tc:=range tests {
  t.Run(tc.name,func(t *testing.T) {
   cfg:=Defaults()
   cfg.Upstream=tc.name
   address,bootstrap,valid:=upstream(cfg)
   if !valid || address!=tc.address || bootstrap!=tc.bootstrap {
    t.Fatalf("unexpected upstream for %s: %q %q %v",tc.name,address,bootstrap,valid)
   }
   yaml,err:=Generate(cfg,"","")
   if err!=nil {t.Fatal(err)}
   for _,item:=range []string{tc.address,tc.bootstrap} {
    if !strings.Contains(yaml,item){t.Fatalf("missing upstream value %s",item)}
   }
  })
 }
}

func TestGeneratedYAMLAgainstOfficialBinary(t *testing.T) {
 binary:=os.Getenv("OXIDNS_CHECK_BINARY")
 if binary==""{t.Skip("set OXIDNS_CHECK_BINARY to verify official OxiDNS parser")}
 for _,provider:=range []string{"quad9","cloudflare","adguard","google","yandex-basic","yandex-safe","yandex-family"} {
  t.Run(provider,func(t *testing.T){
   cfg:=Defaults()
   cfg.Upstream=provider
   payload,err:=Generate(cfg,"","")
   if err!=nil{t.Fatal(err)}
   workDir:=t.TempDir()
   configFile:=filepath.Join(workDir,"config.yaml")
   if err:=os.WriteFile(configFile,[]byte(payload),0600);err!=nil{t.Fatal(err)}
   command:=exec.Command(binary,"check","-c",configFile,"-d",workDir)
   if output,err:=command.CombinedOutput();err!=nil{
    t.Fatalf("official OxiDNS rejected %s: %v\n%s\n%s",provider,err,output,payload)
   }
  })
 }
}
