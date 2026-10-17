package oxidns

import (
 "strings"
 "testing"
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
 if result["rules"]!=original["rules"]{t.Fatal("replaced routing rules")}
 if len(result["servers"].([]map[string]any))!=2{t.Fatal("user server lost")}
 if _,err:=WithSingBoxDNS(result);err==nil{t.Fatal("must reject colliding managed tag")}
}
