package oxidns

import (
 "strings"
 "reflect"
 "testing"
 "os"
 "os/exec"
 "crypto/ecdsa"
 "crypto/elliptic"
 "crypto/rand"
 "crypto/x509"
 "crypto/x509/pkix"
 "encoding/pem"
 "math/big"
 "time"
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

func TestDoHCertificateAndOfficialParser(t *testing.T) {
 workDir:=t.TempDir()
 key,err:=ecdsa.GenerateKey(elliptic.P256(),rand.Reader)
 if err!=nil{t.Fatal(err)}
 cert:=&x509.Certificate{
  SerialNumber:big.NewInt(1),
  Subject:pkix.Name{CommonName:"dns.example.com"},
  DNSNames:[]string{"dns.example.com"},
  NotBefore:time.Now().Add(-time.Hour),
  NotAfter:time.Now().Add(time.Hour),
  KeyUsage:x509.KeyUsageDigitalSignature,
  ExtKeyUsage:[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
 }
 der,err:=x509.CreateCertificate(rand.Reader,cert,cert,&key.PublicKey,key)
 if err!=nil{t.Fatal(err)}
 keyDer,err:=x509.MarshalECPrivateKey(key)
 if err!=nil{t.Fatal(err)}
 certPath:=filepath.Join(workDir,"cert.pem")
 keyPath:=filepath.Join(workDir,"key.pem")
 if err:=os.WriteFile(certPath,pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",Bytes:der}),0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(keyPath,pem.EncodeToMemory(&pem.Block{Type:"EC PRIVATE KEY",Bytes:keyDer}),0600);err!=nil{t.Fatal(err)}
 cfg:=Defaults()
 cfg.DoH=true
 cfg.Domain="dns.example.com"
 yaml,err:=Generate(cfg,certPath,keyPath)
 if err!=nil{t.Fatal(err)}
 if !strings.Contains(yaml,"type: http_server") || !strings.Contains(yaml,"/dns-query") {
  t.Fatal("DoH listener and path were not generated")
 }
 if !strings.Contains(yaml,`listen: ":8443"`) {t.Fatal("unexpected DoH port")}
 cfg.Domain="wrong.example.com"
 if _,err:=Generate(cfg,certPath,keyPath);err==nil{t.Fatal("mismatched certificate accepted")}
 if binary:=os.Getenv("OXIDNS_CHECK_BINARY");binary!=""{
  configPath:=filepath.Join(workDir,"config.yaml")
  if err:=os.WriteFile(configPath,[]byte(yaml),0600);err!=nil{t.Fatal(err)}
  cmd:=exec.Command(binary,"check","-c",configPath,"-d",workDir)
  if output,err:=cmd.CombinedOutput();err!=nil{t.Fatalf("official OxiDNS rejected DoH: %v\n%s\n%s",err,output,yaml)}
 }
}
