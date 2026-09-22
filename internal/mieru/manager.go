package mieru

import (
  "encoding/json"
  "fmt"
  "os"
  "path/filepath"
  "slices"
  "strconv"
  "strings"
  "sync"

  "github.com/SawaMEN/3x-ui/v3/internal/config"
  "github.com/SawaMEN/3x-ui/v3/internal/database/model"
  "github.com/SawaMEN/3x-ui/v3/internal/logger"
)

type PortBinding struct { Port int; Protocol string }
type User struct { Name string; Password string }

type Instance struct {
  Id int
  Tag string
  Listen string
  PortBindings []PortBinding
  Users []User
  MTU int
  LoggingLevel string
  UserHintIsMandatory bool
}

func stringSlice(v any) []string {
  raw, _ := v.([]any)
  out := make([]string, 0, len(raw))
  for _, x := range raw { if s, ok := x.(string); ok { out = append(out, s) } }
  return out
}

func intSlice(v any) []int {
  raw, _ := v.([]any)
  out := make([]int, 0, len(raw))
  for _, x := range raw {
    switch n := x.(type) {
    case float64: out = append(out, int(n))
    case int: out = append(out, n)
    }
  }
  return out
}

func InstanceFromInbound(ib *model.Inbound) (Instance, bool) {
  if ib == nil || ib.Protocol != model.Mieru { return Instance{}, false }
  var raw map[string]any
  if err := json.Unmarshal([]byte(ib.Settings), &raw); err != nil { return Instance{}, false }
  protocols := stringSlice(raw["protocols"])
  if len(protocols) == 0 { protocols = []string{"TCP", "UDP"} }
  ports := append([]int{ib.Port}, intSlice(raw["additionalPorts"])...)
  seen := map[string]struct{}{}
  bindings := make([]PortBinding, 0, len(ports)*len(protocols))
  for _, port := range ports {
    if port < 1 || port > 65535 { continue }
    for _, proto := range protocols {
      proto = strings.ToUpper(strings.TrimSpace(proto))
      if proto != "TCP" && proto != "UDP" { continue }
      key := strconv.Itoa(port)+":"+proto
      if _, ok := seen[key]; ok { continue }
      seen[key] = struct{}{}
      bindings = append(bindings, PortBinding{Port: port, Protocol: proto})
    }
  }
  clients, _ := raw["clients"].([]any)
  users := make([]User, 0, len(clients))
  for _, item := range clients {
    client, _ := item.(map[string]any)
    enabled, _ := client["enable"].(bool)
    email, _ := client["email"].(string)
    password, _ := client["password"].(string)
    if enabled && strings.TrimSpace(email) != "" && password != "" { users = append(users, User{Name: strings.TrimSpace(email), Password: password}) }
  }
  if len(bindings) == 0 || len(users) == 0 { return Instance{}, false }
  mtu, _ := raw["mtu"].(float64)
  if int(mtu) < 1280 || int(mtu) > 1400 { mtu = 1400 }
  level, _ := raw["loggingLevel"].(string)
  level = strings.ToUpper(strings.TrimSpace(level))
  switch level { case "OFF", "ERROR", "WARN", "INFO", "DEBUG": default: level = "INFO" }
  mandatory, _ := raw["userHintIsMandatory"].(bool)
  return Instance{Id:ib.Id, Tag:ib.Tag, Listen:strings.TrimSpace(ib.Listen), PortBindings:bindings, Users:users, MTU:int(mtu), LoggingLevel:level, UserHintIsMandatory:mandatory}, true
}

func (inst Instance) fingerprint() string {
  parts := []string{inst.Listen, strconv.Itoa(inst.MTU), inst.LoggingLevel, strconv.FormatBool(inst.UserHintIsMandatory)}
  for _, b := range inst.PortBindings { parts = append(parts, fmt.Sprintf("%d/%s", b.Port, b.Protocol)) }
  for _, u := range inst.Users { parts = append(parts, u.Name+"="+u.Password) }
  slices.Sort(parts)
  return strings.Join(parts, "|")
}

type managed struct { proc *Process; tag string; fp string }
type Manager struct { mu sync.Mutex; procs map[int]*managed; lastErr map[int]string }
var ( once sync.Once; singleton *Manager )
func GetManager() *Manager { once.Do(func(){ singleton=&Manager{procs:map[int]*managed{}, lastErr:map[int]string{}} }); return singleton }
func configDir() string { return filepath.Join(config.GetBinFolderPath(), "mieru") }
func configPathForID(id int) string { return filepath.Join(configDir(), fmt.Sprintf("mita-%d.json", id)) }
func socketPathForID(id int) string { return filepath.Join(configDir(), fmt.Sprintf("mita-%d.sock", id)) }
func renderConfig(inst Instance) map[string]any {
  bindings:=make([]any,0,len(inst.PortBindings)); for _, b:=range inst.PortBindings { bindings=append(bindings,map[string]any{"port":b.Port,"protocol":b.Protocol}) }
  users:=make([]any,0,len(inst.Users)); for _, u:=range inst.Users { users=append(users,map[string]any{"name":u.Name,"password":u.Password}) }
  out:=map[string]any{"portBindings":bindings,"users":users,"loggingLevel":inst.LoggingLevel,"mtu":inst.MTU}
  if inst.Listen!="" { out["listenIPAddress"]=inst.Listen }
  if inst.UserHintIsMandatory { out["advancedSettings"]=map[string]any{"userHintIsMandatory":true} }
  return out
}
func writeConfig(inst Instance) (string,error) {
  if err:=os.MkdirAll(configDir(),0750); err!=nil{return "",err}
  data,err:=json.MarshalIndent(renderConfig(inst),"","  "); if err!=nil{return "",err}
  path:=configPathForID(inst.Id); if err:=os.WriteFile(path,append(data,'\n'),0640);err!=nil{return "",err}; return path,nil
}
func (m *Manager) ensureLocked(inst Instance) error {
  fp:=inst.fingerprint()
  if cur:=m.procs[inst.Id]; cur!=nil && cur.proc!=nil && cur.proc.IsRunning() && cur.fp==fp { cur.tag=inst.Tag; return nil }
  if cur:=m.procs[inst.Id]; cur!=nil { _=cur.proc.Stop(); delete(m.procs,inst.Id) }
  path,err:=writeConfig(inst); if err!=nil{return err}
  proc:=newProcess(path,inst.Tag,socketPathForID(inst.Id)); if err:=proc.Start();err!=nil{return err}
  m.procs[inst.Id]=&managed{proc:proc,tag:inst.Tag,fp:fp}; delete(m.lastErr,inst.Id)
  logger.Infof("mieru: started mita for inbound %d (%s)",inst.Id,inst.Tag); return nil
}
func (m *Manager) Ensure(inst Instance) error {m.mu.Lock();defer m.mu.Unlock();return m.ensureLocked(inst)}
func (m *Manager) Reconcile(desired []Instance) {
  m.mu.Lock();defer m.mu.Unlock(); want:=map[int]Instance{}; for _,inst:=range desired{want[inst.Id]=inst}
  for id,cur:=range m.procs { if _,ok:=want[id];!ok { _=cur.proc.Stop(); delete(m.procs,id); _=os.Remove(configPathForID(id)); _=os.Remove(socketPathForID(id)) } }
  for _,inst:=range desired { if err:=m.ensureLocked(inst);err!=nil && m.lastErr[inst.Id]!=err.Error(){m.lastErr[inst.Id]=err.Error();logger.Warningf("mieru: failed to start inbound %d (%s): %v",inst.Id,inst.Tag,err)} }
}
func (m *Manager) Remove(id int) {m.mu.Lock();defer m.mu.Unlock();if cur:=m.procs[id];cur!=nil{_ = cur.proc.Stop();delete(m.procs,id)};_ = os.Remove(configPathForID(id));_ = os.Remove(socketPathForID(id))}
func (m *Manager) StopAll() {m.mu.Lock();defer m.mu.Unlock();for id,cur:=range m.procs{_=cur.proc.Stop();_=os.Remove(configPathForID(id));_=os.Remove(socketPathForID(id));delete(m.procs,id)}}
func (m *Manager) HasRunning() bool {m.mu.Lock();defer m.mu.Unlock();for _,cur:=range m.procs{if cur.proc!=nil&&cur.proc.IsRunning(){return true}};return false}