package service

import (
 "encoding/json"
 "fmt"
 "strings"

 "github.com/SawaMEN/3x-ui/v3/internal/singbox"
 "github.com/SawaMEN/3x-ui/v3/internal/xray"
 "github.com/SawaMEN/3x-ui/v3/internal/zapret2"
 "github.com/SawaMEN/3x-ui/v3/internal/util/json_util"
)

const zapret2TagPrefix = "__xui_zapret2_"

func zapret2Tags(tag string) (string,string) {
 return zapret2TagPrefix+"listen_"+tag,zapret2TagPrefix+"direct_"+tag
}

func parseZapret2Outbounds(rows []map[string]any) ([]map[string]any,[]zapret2.Plan,error) {
 pending:=make([]zapret2.Plan,0)
 for _,row:=range rows {
  if !strings.EqualFold(fmt.Sprint(row["protocol"]),"zapret2") {continue}
  if !zapret2.Installed() {return nil,nil,fmt.Errorf("zapret2 is not installed; install the official component in Settings before saving this outbound")}
  tag,_:=row["tag"].(string)
  plan,err:=zapret2.Parse(tag,row["settings"])
  if err!=nil{return nil,nil,err}
  if strings.HasPrefix(tag,zapret2TagPrefix) {return nil,nil,fmt.Errorf("zapret2 outbound tag uses a reserved prefix")}
  pending=append(pending,plan)
 }
 plans,err:=zapret2.AssignQueues(pending)
 if err!=nil{return nil,nil,err}
 index:=make(map[string]zapret2.Plan,len(plans))
 for _,p:=range plans {index[p.Tag]=p}
 // The first outbound is Xray's implicit default. Keep every virtual
 // Anti-DPI outbound after the existing non-zapret2 outbounds, regardless of
 // the order chosen in the editor.
 translated:=make([]map[string]any,0,len(rows)+1)
 virtual:=make([]map[string]any,0,len(plans))
 for _,row:=range rows {
  if !strings.EqualFold(fmt.Sprint(row["protocol"]),"zapret2") {
   translated=append(translated,row)
   continue
  }
  p:=index[fmt.Sprint(row["tag"])]
  virtual=append(virtual,map[string]any{
   "protocol":"socks","tag":p.Tag,
   "settings":map[string]any{"servers":[]any{map[string]any{"address":"127.0.0.1","port":p.Options.Port}}},
  })
 }
 if len(virtual)>0 && len(translated)==0 {
  translated=append(translated,map[string]any{
   "protocol":"freedom","tag":"direct","settings":map[string]any{},
  })
 }
 translated=append(translated,virtual...)
 return translated,plans,nil
}

func injectXrayZapret2(cfg *xray.Config) error {
 if cfg==nil||len(cfg.OutboundConfigs)==0{return nil}
 var original []map[string]any
 if err:=json.Unmarshal(cfg.OutboundConfigs,&original);err!=nil{return err}
 translated,plans,err:=parseZapret2Outbounds(original)
 if err!=nil{return err}
 if len(plans)==0{return nil}
 router:=map[string]any{}
 if len(cfg.RouterConfig)>0{
  if err:=json.Unmarshal(cfg.RouterConfig,&router);err!=nil{return err}
 }
 occupied:=map[string]bool{}
 for _,out:=range translated {occupied[fmt.Sprint(out["tag"])]=true}
 for _,in:=range cfg.InboundConfigs {occupied[in.Tag]=true}
 for _,p:=range plans {
  inTag,outTag:=zapret2Tags(p.Tag)
  if occupied[inTag]||occupied[outTag]{return fmt.Errorf("zapret2 %q: reserved inbound/outbound tag collision",p.Tag)}
  occupied[inTag]=true;occupied[outTag]=true
  for _,in:=range cfg.InboundConfigs {
   if in.Port==p.Options.Port{return fmt.Errorf("zapret2 local port %d conflicts with an inbound",p.Options.Port)}
  }
  cfg.InboundConfigs=append(cfg.InboundConfigs,xray.InboundConfig{
   Listen:json_util.RawMessage(`"127.0.0.1"`),Port:p.Options.Port,
   Protocol:"socks",Tag:inTag,
   Settings:json_util.RawMessage(`{"auth":"noauth","udp":true,"ip":"127.0.0.1"}`),
  })
  translated=append(translated,map[string]any{
   "protocol":"freedom","tag":outTag,"settings":map[string]any{},
   "streamSettings":map[string]any{"sockopt":map[string]any{"mark":p.Mark}},
  })
  rawRules,_:=router["rules"].([]any)
  router["rules"]=append([]any{map[string]any{
   "type":"field","inboundTag":[]string{inTag},"outboundTag":outTag,
  }},rawRules...)
 }
 data,err:=json.Marshal(translated);if err!=nil{return err}
 routes,err:=json.Marshal(router);if err!=nil{return err}
 cfg.OutboundConfigs=json_util.RawMessage(data)
 cfg.RouterConfig=json_util.RawMessage(routes)
 cfg.Zapret2Plans=plans
 return nil
}

func injectSingBoxZapret2(cfg *singbox.Config) error {
 if cfg==nil||len(cfg.Zapret2Plans)==0{return nil}
 plans,err:=zapret2.AssignQueues(cfg.Zapret2Plans)
 if err!=nil{return err}
 occupied:=map[string]bool{}
 for _,out:=range cfg.Outbounds {occupied[fmt.Sprint(out["tag"])]=true}
 for _,in:=range cfg.Inbounds {occupied[fmt.Sprint(in["tag"])]=true}
 for _,p:=range plans {
  inTag,outTag:=zapret2Tags(p.Tag)
  if occupied[inTag]||occupied[outTag] {return fmt.Errorf("zapret2 %q: reserved tag collision",p.Tag)}
  occupied[inTag]=true;occupied[outTag]=true
  for _,in:=range cfg.Inbounds {
   if fmt.Sprint(in["listen_port"])==fmt.Sprint(p.Options.Port){return fmt.Errorf("zapret2 port %d conflicts with an inbound",p.Options.Port)}
  }
  cfg.Inbounds=append(cfg.Inbounds,map[string]any{
   "type":"socks","tag":inTag,"listen":"127.0.0.1","listen_port":p.Options.Port,
  })
  cfg.Outbounds=append(cfg.Outbounds,map[string]any{
   "type":"direct","tag":outTag,"routing_mark":p.Mark,
  })
  if cfg.Route==nil{cfg.Route=map[string]any{}}
  rawRules:=singBoxRouteRules(cfg.Route)
  cfg.Route["rules"]=append([]any{map[string]any{
   "inbound":[]string{inTag},"outbound":outTag,
  }},rawRules...)
 }
 // sing-box also defaults to the first panel outbound. Never let a
 // virtual zapret2 route become the implicit fallback for unmatched traffic.
 if cfg.Route!=nil {
  for _,p:=range plans {
   if cfg.Route["final"]==p.Tag {
    replacement:=""
    for _,out:=range cfg.Outbounds {
     tag,_:=out["tag"].(string)
     if tag=="direct" {replacement="direct";break}
    }
    if replacement=="" {
     for _,out:=range cfg.Outbounds {
      tag,_:=out["tag"].(string)
      if tag!=p.Tag && !strings.HasPrefix(tag,zapret2TagPrefix) {
       replacement=tag
       break
      }
     }
    }
    if replacement=="" {return fmt.Errorf("zapret2 %q cannot be the only default outbound",p.Tag)}
    cfg.Route["final"]=replacement
    break
   }
  }
 }
 cfg.Zapret2Plans=plans
 return nil
}
