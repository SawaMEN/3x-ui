package mtproto

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

type SecretEntry struct {
	Name        string
	Secret      string
	AdTag       string
	QuotaBytes  int64
	ExpiresUnix int64
}

type Instance struct {
	Id      int
	Tag     string
	Listen  string
	Port    int
	Secrets []SecretEntry

	Debug                  bool
	ProxyProtocolListener  bool
	PreferIP               string
	FrontingIP             string
	FrontingPort           int
	FrontingProxyProtocol  bool
	ThrottleMaxConnections int
	PublicIPv4             string
	PublicIPv6             string
	RouteThroughXray       bool
	XrayRoutePort          int
	FakeTLSDomain          string
}

func (inst Instance) bindTo() string {
	listen := strings.TrimSpace(inst.Listen)
	if listen == "" { listen = "0.0.0.0" }
	return fmt.Sprintf("%s:%d", listen, inst.Port)
}
func (inst Instance) structuralFingerprint() string {
	return strings.Join([]string{inst.bindTo(), strconv.FormatBool(inst.Debug), strconv.FormatBool(inst.ProxyProtocolListener), inst.PreferIP, inst.FrontingIP, strconv.Itoa(inst.FrontingPort), strconv.FormatBool(inst.FrontingProxyProtocol), strconv.Itoa(inst.ThrottleMaxConnections), strconv.FormatBool(inst.RouteThroughXray), strconv.Itoa(inst.XrayRoutePort), inst.PublicIPv4, inst.PublicIPv6, inst.FakeTLSDomain}, "|")
}
func (inst Instance) secretsFingerprint() string {
	pairs := make([]string, 0, len(inst.Secrets))
	for _, e := range inst.Secrets { pairs = append(pairs, fmt.Sprintf("%s=%s;tag=%s;q=%d;exp=%d", e.Name, e.Secret, e.AdTag, e.QuotaBytes, e.ExpiresUnix)) }
	slices.Sort(pairs); return strings.Join(pairs, "|")
}

type Traffic struct { Tag, Email string; Up, Down int64 }
type clientCounters struct { up, down int64 }
func monotonicCounterDelta(current, previous int64) int64 { if current <= 0 { return 0 }; if current >= previous { return current-previous }; return current }

type managed struct { proc *Process; tag, structuralFP, secretsFP string; apiPort int; apiToken string; last map[string]clientCounters }
type Manager struct { mu sync.Mutex; procs map[int]*managed; swept bool }
var ( managerOnce sync.Once; manager *Manager )
func GetManager() *Manager { managerOnce.Do(func(){ manager=&Manager{procs:map[int]*managed{}} }); return manager }

// decodeLegacySecret converts an mtg Telegram-link secret to Telemt's raw
// 32-hex user secret and recovers the FakeTLS host embedded after an ee secret.
func decodeLegacySecret(secret string) (raw, domain string) {
	s := strings.TrimSpace(secret)
	if len(s) >= 34 && (strings.HasPrefix(s,"ee") || strings.HasPrefix(s,"dd")) {
		raw=s[2:34]
		if strings.HasPrefix(s,"ee") && len(s)>34 { if b,err:=hex.DecodeString(s[34:]); err==nil { domain=strings.TrimSpace(string(b)) } }
		return raw,domain
	}
	if len(s)>=32 { return s[:32],"" }
	return s,""
}

func InstanceFromInbound(ib *model.Inbound) (Instance,bool) {
	if ib==nil || ib.Protocol!=model.MTProto { return Instance{},false }
	var parsed struct {
		ProxyProtocolListener bool `json:"proxyProtocolListener"`; Debug bool `json:"debug"`; FakeTLSDomain string `json:"fakeTlsDomain"`
		DomainFronting struct { IP string `json:"ip"`; Port int `json:"port"`; ProxyProtocol bool `json:"proxyProtocol"` } `json:"domainFronting"`
		PreferIP string `json:"preferIp"`; ThrottleMaxConnections int `json:"throttleMaxConnections"`; RouteThroughXray bool `json:"routeThroughXray"`; RouteXrayPort int `json:"routeXrayPort"`; PublicIPv4 string `json:"publicIpv4"`; PublicIPv6 string `json:"publicIpv6"`
		Clients []struct { Email string `json:"email"`; Secret string `json:"secret"`; AdTag string `json:"adTag"`; Enable bool `json:"enable"`; TotalGB int64 `json:"totalGB"`; ExpiryTime int64 `json:"expiryTime"` } `json:"clients"`
	}
	if err:=json.Unmarshal([]byte(ib.Settings),&parsed);err!=nil{return Instance{},false}
	secrets:=make([]SecretEntry,0,len(parsed.Clients)); domain:=strings.TrimSpace(parsed.FakeTLSDomain)
	for _,c:=range parsed.Clients {
		if !c.Enable || c.Secret=="" || c.Email=="" { continue }
		raw,embeddedDomain:=decodeLegacySecret(c.Secret); if len(raw)!=32 { continue }; if domain=="" && embeddedDomain!="" { domain=embeddedDomain }
		e:=SecretEntry{Name:c.Email,Secret:raw,AdTag:usableAdTag(c.AdTag)}; if c.TotalGB>0 {e.QuotaBytes=c.TotalGB}; if c.ExpiryTime>0 {e.ExpiresUnix=c.ExpiryTime/1000}; secrets=append(secrets,e)
	}
	if len(secrets)==0{return Instance{},false}
	return Instance{Id:ib.Id,Tag:ib.Tag,Listen:ib.Listen,Port:ib.Port,Secrets:secrets,Debug:parsed.Debug,ProxyProtocolListener:parsed.ProxyProtocolListener,PreferIP:parsed.PreferIP,FrontingIP:parsed.DomainFronting.IP,FrontingPort:parsed.DomainFronting.Port,FrontingProxyProtocol:parsed.DomainFronting.ProxyProtocol,ThrottleMaxConnections:parsed.ThrottleMaxConnections,RouteThroughXray:parsed.RouteThroughXray,XrayRoutePort:parsed.RouteXrayPort,PublicIPv4:strings.TrimSpace(parsed.PublicIPv4),PublicIPv6:strings.TrimSpace(parsed.PublicIPv6),FakeTLSDomain:domain},true
}
func usableAdTag(tag string) string { tag=strings.TrimSpace(tag); if !model.ValidMtprotoAdTag(tag){return ""}; return tag }

func (m *Manager) Ensure(inst Instance) error {m.mu.Lock();defer m.mu.Unlock();m.sweepOrphansLocked();return m.ensureLocked(inst)}
func (m *Manager) sweepOrphansLocked(){if m.swept{return};m.swept=true;if n:=killStrayMtgProcesses(GetBinaryPath());n>0{logger.Warningf("mtproto: terminated %d orphaned Telemt process(es)",n)}}
type ensureAction int
const(ensureNoop ensureAction=iota;ensureReload;ensureRestart)
func ensureActionFor(running bool,curStructFP,curSecretsFP,newStructFP,newSecretsFP string)ensureAction{if !running||curStructFP!=newStructFP{return ensureRestart};if curSecretsFP!=newSecretsFP{return ensureReload};return ensureNoop}
func(m *Manager)ensureLocked(inst Instance)error{
	structFP,secFP:=inst.structuralFingerprint(),inst.secretsFingerprint()
	if cur,ok:=m.procs[inst.Id];ok{switch ensureActionFor(cur.proc.IsRunning(),cur.structuralFP,cur.secretsFP,structFP,secFP){case ensureNoop:cur.tag=inst.Tag;return nil;case ensureReload:if err:=writeConfig(configPathForID(inst.Id),inst,cur.apiPort,cur.apiToken);err!=nil{return err};if reloadTelemt(cur.apiPort,cur.apiToken){cur.tag=inst.Tag;cur.secretsFP=secFP;logger.Infof("mtproto: reloaded Telemt users for inbound %d",inst.Id);return nil};logger.Warningf("mtproto: Telemt reload failed for inbound %d, restarting",inst.Id);fallthrough;case ensureRestart:_=cur.proc.Stop();delete(m.procs,inst.Id)}}
	apiPort,err:=FreeLocalPort();if err!=nil{return err};apiToken,err:=newAPIToken();if err!=nil{return err};cfg:=configPathForID(inst.Id);if err:=writeConfig(cfg,inst,apiPort,apiToken);err!=nil{return err};proc:=newProcess(cfg,fmt.Sprintf("inbound %d",inst.Id));if err:=proc.Start();err!=nil{return err};m.procs[inst.Id]=&managed{proc:proc,tag:inst.Tag,structuralFP:structFP,secretsFP:secFP,apiPort:apiPort,apiToken:apiToken,last:map[string]clientCounters{}};logger.Infof("mtproto: started Telemt for inbound %d on %s",inst.Id,inst.bindTo());return nil
}
func(m *Manager)Remove(id int){m.mu.Lock();defer m.mu.Unlock();if cur,ok:=m.procs[id];ok{_=cur.proc.Stop();delete(m.procs,id);_=os.Remove(configPathForID(id));logger.Infof("mtproto: stopped Telemt for inbound %d",id)}}
func(m *Manager)Reconcile(desired []Instance){m.mu.Lock();defer m.mu.Unlock();m.sweepOrphansLocked();want:=make(map[int]struct{},len(desired));for _,i:=range desired{want[i.Id]=struct{}{}};for id,cur:=range m.procs{if _,ok:=want[id];!ok{_=cur.proc.Stop();delete(m.procs,id);_=os.Remove(configPathForID(id))}};for _,i:=range desired{if err:=m.ensureLocked(i);err!=nil{logger.Warningf("mtproto: reconcile failed for inbound %d: %v",i.Id,err)}}}
func(m *Manager)StopAll(){m.mu.Lock();defer m.mu.Unlock();for id,cur:=range m.procs{_=cur.proc.Stop();_=os.Remove(configPathForID(id));delete(m.procs,id)}}

func(m *Manager)CollectTraffic()([]Traffic,[]string){
	type snap struct{id,apiPort int;apiToken,tag string;last map[string]clientCounters};m.mu.Lock();snaps:=make([]snap,0,len(m.procs));for id,cur:=range m.procs{if cur.proc==nil||!cur.proc.IsRunning(){continue};cp:=make(map[string]clientCounters,len(cur.last));maps.Copy(cp,cur.last);snaps=append(snaps,snap{id,cur.apiPort,cur.apiToken,cur.tag,cp})};m.mu.Unlock();var out []Traffic;var online []string
	for _,s:=range snaps{users,ok:=scrapeStats(s.apiPort,s.apiToken);if !ok{continue};next:=make(map[string]clientCounters,len(users));for email,u:=range users{total:=u.TotalOctets;next[email]=clientCounters{down:total};if u.CurrentConnections>0{online=append(online,email)};if prev,had:=s.last[email];had{d:=monotonicCounterDelta(total,prev.down);if d>0{out=append(out,Traffic{Tag:s.tag,Email:email,Down:d})}}};m.mu.Lock();if cur,ok:=m.procs[s.id];ok{cur.last=next};m.mu.Unlock()};return out,online
}
func(m *Manager)HasRunning()bool{m.mu.Lock();defer m.mu.Unlock();for _,cur:=range m.procs{if cur.proc!=nil&&cur.proc.IsRunning(){return true}};return false}
func(m *Manager)ResetQuota(email string){if email==""{return};type target struct{port int;token string};m.mu.Lock();ts:=make([]target,0,len(m.procs));for _,cur:=range m.procs{if cur.proc!=nil&&cur.proc.IsRunning(){ts=append(ts,target{cur.apiPort,cur.apiToken})}};m.mu.Unlock();for _,t:=range ts{resetQuota(t.port,t.token,email)}}
func resetQuota(port int,token,email string){req,err:=http.NewRequestWithContext(context.Background(),http.MethodPost,fmt.Sprintf("http://127.0.0.1:%d/v1/users/%s/reset-quota",port,url.PathEscape(email)),nil);if err!=nil{return};authorize(req,token);resp,err:=(&http.Client{Timeout:3*time.Second}).Do(req);if err==nil{_=resp.Body.Close()}}
func FreeLocalPort()(int,error){l,err:=(&net.ListenConfig{}).Listen(context.Background(),"tcp","127.0.0.1:0");if err!=nil{return 0,err};defer l.Close();return l.Addr().(*net.TCPAddr).Port,nil}

func tomlQuote(s string)string{return strconv.Quote(s)}
func renderConfig(inst Instance,apiPort int,apiToken string)string{
	var b strings.Builder;b.WriteString("[general]\nfast_mode = true\nuse_middle_proxy = false\n");if inst.Debug{b.WriteString("log_level = \"debug\"\n")}else{b.WriteString("log_level = \"normal\"\n")};b.WriteString("\n[general.modes]\nclassic = false\nsecure = false\ntls = true\n");b.WriteString("\n[network]\nipv4 = true\nipv6 = true\n");switch strings.ToLower(inst.PreferIP){case"only-ipv6","prefer-ipv6":b.WriteString("prefer = 6\n");default:b.WriteString("prefer = 4\n")};fmt.Fprintf(&b,"\n[server]\nport = %d\n",inst.Port);listen:=strings.TrimSpace(inst.Listen);if listen==""{listen="0.0.0.0"};fmt.Fprintf(&b,"\n[[server.listeners]]\nip = %s\n",tomlQuote(listen));fmt.Fprintf(&b,"\n[server.api]\nenabled = true\nlisten = %s\nwhitelist = [\"127.0.0.0/8\"]\nauth_header = %s\nread_only = false\n",tomlQuote(fmt.Sprintf("127.0.0.1:%d",apiPort)),tomlQuote("Bearer "+apiToken));if inst.FakeTLSDomain!=""{fmt.Fprintf(&b,"\n[censorship]\ntls_domain = %s\nmask = true\ntls_emulation = true\n",tomlQuote(inst.FakeTLSDomain))};b.WriteString("\n[access]\nreplay_check_len = 65536\nignore_time_skew = false\n");if inst.ThrottleMaxConnections>0{fmt.Fprintf(&b,"user_max_tcp_conns_global_each = %d\n",inst.ThrottleMaxConnections)};b.WriteString("\n[access.users]\n");for _,e:=range inst.Secrets{fmt.Fprintf(&b,"%s = %s\n",tomlQuote(e.Name),tomlQuote(e.Secret))};tagged:=false;for _,e:=range inst.Secrets{if e.AdTag!=""{if !tagged{b.WriteString("\n[access.user_ad_tags]\n");tagged=true};fmt.Fprintf(&b,"%s = %s\n",tomlQuote(e.Name),tomlQuote(e.AdTag))}};quota:=false;for _,e:=range inst.Secrets{if e.QuotaBytes>0{if !quota{b.WriteString("\n[access.user_data_quota]\n");quota=true};fmt.Fprintf(&b,"%s = %d\n",tomlQuote(e.Name),e.QuotaBytes)}};exp:=false;for _,e:=range inst.Secrets{if e.ExpiresUnix>0{if !exp{b.WriteString("\n[access.user_expirations]\n");exp=true};fmt.Fprintf(&b,"%s = %s\n",tomlQuote(e.Name),tomlQuote(expiresString(e.ExpiresUnix)))}};if inst.RouteThroughXray&&inst.XrayRoutePort>0{fmt.Fprintf(&b,"\n[[upstreams]]\ntype = \"socks5\"\naddress = %s\nweight = 1\nenabled = true\n",tomlQuote(fmt.Sprintf("127.0.0.1:%d",inst.XrayRoutePort)))}else{b.WriteString("\n[[upstreams]]\ntype = \"direct\"\nweight = 1\nenabled = true\n")};return b.String()
}
func writeConfig(path string,inst Instance,apiPort int,apiToken string)error{if err:=os.MkdirAll(configDir(),0750);err!=nil{return err};return os.WriteFile(path,[]byte(renderConfig(inst,apiPort,apiToken)),0640)}
func expiresString(unix int64)string{return time.Unix(unix,0).UTC().Format(time.RFC3339)}
func quotaString(bytes int64)string{return strconv.FormatInt(bytes,10)+"B"}
func newAPIToken()(string,error){buf:=make([]byte,16);if _,err:=rand.Read(buf);err!=nil{return"",err};return hex.EncodeToString(buf),nil}
func authorize(req *http.Request,token string){if token!=""{req.Header.Set("Authorization","Bearer "+token)}}
func reloadTelemt(port int,token string)bool{req,err:=http.NewRequestWithContext(context.Background(),http.MethodPost,fmt.Sprintf("http://127.0.0.1:%d/v1/system/reload",port),nil);if err!=nil{return false};authorize(req,token);resp,err:=(&http.Client{Timeout:4*time.Second}).Do(req);if err!=nil{return false};defer resp.Body.Close();return resp.StatusCode==http.StatusAccepted||resp.StatusCode==http.StatusOK}

type secretPutEntry struct{Secret string `json:"secret"`;AdTag string `json:"ad_tag,omitempty"`;Quota string `json:"quota,omitempty"`;Expires string `json:"expires,omitempty"`}
type secretsPutBody struct{Secrets map[string]secretPutEntry `json:"secrets"`}
func secretsPayload(inst Instance)secretsPutBody{m:=make(map[string]secretPutEntry,len(inst.Secrets));for _,e:=range inst.Secrets{x:=secretPutEntry{Secret:e.Secret,AdTag:e.AdTag};if e.QuotaBytes>0{x.Quota=quotaString(e.QuotaBytes)};if e.ExpiresUnix>0{x.Expires=expiresString(e.ExpiresUnix)};m[e.Name]=x};return secretsPutBody{Secrets:m}}
func applySecrets(port int,token string,inst Instance)bool{return reloadTelemt(port,token)}
type statsUser struct{CurrentConnections int64 `json:"current_connections"`;TotalOctets int64 `json:"total_octets"`}
func scrapeStats(port int,token string)(map[string]statsUser,bool){req,err:=http.NewRequestWithContext(context.Background(),http.MethodGet,fmt.Sprintf("http://127.0.0.1:%d/v1/stats/users",port),nil);if err!=nil{return nil,false};authorize(req,token);resp,err:=(&http.Client{Timeout:3*time.Second}).Do(req);if err!=nil{return nil,false};defer resp.Body.Close();if resp.StatusCode!=http.StatusOK{return nil,false};var env struct{OK bool `json:"ok"`;Data []struct{Username string `json:"username"`;CurrentConnections int64 `json:"current_connections"`;TotalOctets int64 `json:"total_octets"`} `json:"data"`};if err:=json.NewDecoder(resp.Body).Decode(&env);err!=nil{return nil,false};out:=make(map[string]statsUser,len(env.Data));for _,u:=range env.Data{out[u.Username]=statsUser{CurrentConnections:u.CurrentConnections,TotalOctets:u.TotalOctets}};return out,true}
