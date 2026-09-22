package psiphon

import (
  "encoding/json"
  "fmt"
  "os"
  "path/filepath"
  "strings"
  "sync"

  "github.com/SawaMEN/3x-ui/v3/internal/config"
  "github.com/SawaMEN/3x-ui/v3/internal/database"
  "github.com/SawaMEN/3x-ui/v3/internal/database/model"
  "github.com/SawaMEN/3x-ui/v3/internal/logger"
)

type Instance struct { Id int; Tag string; ServerAddress string; Protocol string; Port int; ServerEntry string; AdditionalArguments []string }
func InstanceFromInbound(ib *model.Inbound)(Instance,bool){
  if ib==nil||ib.Protocol!=model.Psiphon{return Instance{},false};var raw map[string]any;if json.Unmarshal([]byte(ib.Settings),&raw)!=nil{return Instance{},false}
  addr,_:=raw["serverAddress"].(string);addr=strings.TrimSpace(addr);proto,_:=raw["tunnelProtocol"].(string);proto=strings.ToUpper(strings.TrimSpace(proto));if proto==""{proto="OSSH"}
  entry,_:=raw["serverEntry"].(string);args:=[]string{};if a,ok:=raw["additionalArguments"].([]any);ok{for _,v:=range a{if s,ok:=v.(string);ok&&strings.TrimSpace(s)!=""{args=append(args,s)}}}
  if ib.Port<1||ib.Port>65535|| (addr=="" && strings.TrimSpace(entry)==""){return Instance{},false};return Instance{Id:ib.Id,Tag:ib.Tag,ServerAddress:addr,Protocol:proto,Port:ib.Port,ServerEntry:entry,AdditionalArguments:args},true
}
func (inst Instance) fingerprint()string{return fmt.Sprintf("%s|%s|%d|%s|%s",inst.ServerAddress,inst.Protocol,inst.Port,inst.ServerEntry,strings.Join(inst.AdditionalArguments,"\x00"))}

type managed struct{proc *Process;tag,fp string}
type Manager struct{mu sync.Mutex;procs map[int]*managed;lastErr map[int]string}
var(once sync.Once;singleton *Manager)
func GetManager()*Manager{once.Do(func(){singleton=&Manager{procs:map[int]*managed{},lastErr:map[int]string{}}});return singleton}
func configDir()string{return filepath.Join(config.GetBinFolderPath(),"psiphon")}
func configPathForID(id int)string{return filepath.Join(configDir(),fmt.Sprintf("inbound-%d",id))}
func persistServerEntry(id int,entry string){
  if strings.TrimSpace(entry)==""{return};db:=database.GetDB();if db==nil{return};var ib model.Inbound;if db.Where("id = ?",id).First(&ib).Error!=nil{return};var raw map[string]any;if json.Unmarshal([]byte(ib.Settings),&raw)!=nil{return};raw["serverEntry"]=entry;b,err:=json.Marshal(raw);if err!=nil{return};_ = db.Model(&ib).Update("settings",string(b)).Error
}
func (m *Manager)ensureLocked(inst Instance)error{
  fp:=inst.fingerprint();if cur:=m.procs[inst.Id];cur!=nil&&cur.proc!=nil&&cur.proc.IsRunning()&&cur.fp==fp{cur.tag=inst.Tag;return nil}
  if cur:=m.procs[inst.Id];cur!=nil{_=cur.proc.Stop();delete(m.procs,inst.Id)}
  _=os.RemoveAll(configPathForID(inst.Id))
  if err:=os.MkdirAll(configPathForID(inst.Id),0750);err!=nil{return err}
  proc:=newProcess(configPathForID(inst.Id),inst.Tag,inst);if err:=proc.Start();err!=nil{return err}
  if entry:=proc.ServerEntry();entry!=""{
    if inst.ServerEntry == "" {
      persistServerEntry(inst.Id,entry)
      inst.ServerEntry = entry
      fp = inst.fingerprint()
    }
  }
  m.procs[inst.Id]=&managed{proc:proc,tag:inst.Tag,fp:fp};delete(m.lastErr,inst.Id);logger.Infof("psiphon: started psiphond for inbound %d (%s)",inst.Id,inst.Tag);return nil
}
func(m *Manager)Ensure(inst Instance)error{m.mu.Lock();defer m.mu.Unlock();return m.ensureLocked(inst)}
func(m *Manager)Reconcile(desired []Instance){m.mu.Lock();defer m.mu.Unlock();want:=map[int]Instance{};for _,i:=range desired{want[i.Id]=i};for id,cur:=range m.procs{if _,ok:=want[id];!ok{_=cur.proc.Stop();delete(m.procs,id);_ = os.RemoveAll(configPathForID(id))}};for _,i:=range desired{if err:=m.ensureLocked(i);err!=nil&&m.lastErr[i.Id]!=err.Error(){m.lastErr[i.Id]=err.Error();logger.Warningf("psiphon: failed to start inbound %d (%s): %v",i.Id,i.Tag,err)}}}
func(m *Manager)Remove(id int){m.mu.Lock();defer m.mu.Unlock();if cur:=m.procs[id];cur!=nil{_=cur.proc.Stop();delete(m.procs,id)};_=os.RemoveAll(configPathForID(id))}
func(m *Manager)StopAll(){m.mu.Lock();defer m.mu.Unlock();for id,cur:=range m.procs{_=cur.proc.Stop();_=os.RemoveAll(configPathForID(id));delete(m.procs,id)}}
func(m *Manager)HasRunning()bool{m.mu.Lock();defer m.mu.Unlock();for _,cur:=range m.procs{if cur.proc!=nil&&cur.proc.IsRunning(){return true}};return false}