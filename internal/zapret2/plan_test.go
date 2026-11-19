package zapret2

import (
 "strings"
 "testing"
)

func TestVirtualOutboundSettings(t *testing.T) {
 p,err:=Parse("z2-rf",map[string]any{"port":18080,"profile":"balanced","tcp":true,"udp":true})
 if err!=nil{t.Fatal(err)}
 if p.Options.Port!=18080 || !p.Options.UDP {t.Fatalf("unexpected options: %+v",p.Options)}
 if _,err:=Parse("bad",map[string]any{"port":80});err==nil{t.Fatal("privileged local port was accepted")}
 if _,err:=Parse("bad",map[string]any{"profile":"untrusted"});err==nil{t.Fatal("unknown strategy was accepted")}
}

func TestZapret2QueueIsIsolatedAndDeterministic(t *testing.T) {
 a,_:=Parse("a",map[string]any{"port":16001})
 b,_:=Parse("b",map[string]any{"port":16002})
 one,err:=AssignQueues([]Plan{b,a})
 if err!=nil{t.Fatal(err)}
 two,err:=AssignQueues([]Plan{a,b})
 if err!=nil{t.Fatal(err)}
 if one[0].Tag!="a"||two[0].Queue!=one[0].Queue||one[1].Mark==one[0].Mark {
  t.Fatalf("queue/mark identity not stable: %+v %+v",one,two)
 }
 if _,err:=AssignQueues([]Plan{a,a});err==nil{t.Fatal("duplicate tag accepted")}
 if _,err:=AssignQueues([]Plan{a,{Tag:"c",Options:a.Options}});err==nil{t.Fatal("duplicate port accepted")}
}

func TestZapret2RulesOnlyMatchMarkedEgress(t *testing.T) {
 p,_:=Parse("z2",map[string]any{"port":18080,"profile":"safe","tcp":true,"udp":false})
 plans,_:=AssignQueues([]Plan{p})
 rule:=firewallRules(plans)
 if !strings.Contains(rule,"meta mark 0x72000001")||!strings.Contains(rule,"queue num 2400 bypass")||strings.Contains(rule,"udp dport") {
  t.Fatalf("unexpected nftables rules: %s",rule)
 }
 args,err:=strategyArgs(plans[0])
 if err!=nil||len(args)<4{t.Fatalf("invalid strategy args: %v %v",args,err)}
}
