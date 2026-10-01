package mtproto

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("TELEMT_FAKE_CHILD") == "1" {
		if f,err:=os.OpenFile(os.Getenv("TELEMT_FAKE_PIDFILE"),os.O_APPEND|os.O_CREATE|os.O_WRONLY,0644);err==nil{fmt.Fprintf(f,"%d\n",os.Getpid());_ = f.Close()}
		select{}
	}
	os.Exit(m.Run())
}

func installFakeTelemt(t *testing.T) string {
	t.Helper(); binDir:=t.TempDir(); self,err:=os.Executable(); if err!=nil{t.Fatal(err)}; payload,err:=os.ReadFile(self);if err!=nil{t.Fatal(err)}
	if err:=os.WriteFile(filepath.Join(binDir,GetBinaryName()),payload,0755);err!=nil{t.Fatal(err)}
	pidFile:=filepath.Join(binDir,"telemt-pids.txt");t.Setenv("XUI_BIN_FOLDER",binDir);t.Setenv("TELEMT_FAKE_CHILD","1");t.Setenv("TELEMT_FAKE_PIDFILE",pidFile);return pidFile
}
func spawnCount(t *testing.T,p string)int{t.Helper();b,err:=os.ReadFile(p);if os.IsNotExist(err){return 0};if err!=nil{t.Fatal(err)};return len(strings.Fields(string(b)))}
func waitSpawnCount(t *testing.T,p string,want int){t.Helper();deadline:=time.Now().Add(3*time.Second);for{if got:=spawnCount(t,p);got==want{return}else if got>want{t.Fatalf("spawns=%d want=%d",got,want)};if time.Now().After(deadline){t.Fatalf("spawn timeout: got %d want %d",spawnCount(t,p),want)};time.Sleep(20*time.Millisecond)}}
func telemtInst(id int,secrets ...SecretEntry)Instance{return Instance{Id:id,Tag:fmt.Sprintf("inbound-%d",id),Listen:"127.0.0.1",Port:24000+id,FakeTLSDomain:"example.com",Secrets:secrets}}

func TestEnsureActionFor(t *testing.T){
	if ensureActionFor(false,"s","a","s","a")!=ensureRestart{t.Fatal("dead process must restart")}
	if ensureActionFor(true,"s1","a","s2","a")!=ensureRestart{t.Fatal("structural change must restart")}
	if ensureActionFor(true,"s","a","s","b")!=ensureReload{t.Fatal("user change must reload")}
	if ensureActionFor(true,"s","a","s","a")!=ensureNoop{t.Fatal("unchanged must be noop")}
}

func TestReloadTelemt(t *testing.T){
	var method,path,auth string
	srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){method,path,auth=r.Method,r.URL.Path,r.Header.Get("Authorization");w.WriteHeader(http.StatusAccepted)}));defer srv.Close()
	if !reloadTelemt(serverPort(t,srv),"sesame"){t.Fatal("reload should accept 202")}
	if method!=http.MethodPost||path!="/v1/system/reload"||auth!="Bearer sesame"{t.Fatalf("bad reload request: %s %s %q",method,path,auth)}
}

func TestEnsureHotReloadKeepsProcess(t *testing.T){
	pidFile:=installFakeTelemt(t);mgr:=&Manager{procs:map[int]*managed{},swept:true}
	inst:=telemtInst(1,SecretEntry{Name:"alice",Secret:"0123456789abcdef0123456789abcdef"});if err:=mgr.Ensure(inst);err!=nil{t.Fatalf("initial ensure: %v",err)};waitSpawnCount(t,pidFile,1);orig:=mgr.procs[1].proc;token:=mgr.procs[1].apiToken
	reloaded:=make(chan struct{},1);srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){if r.Method==http.MethodPost&&r.URL.Path=="/v1/system/reload"{reloaded<-struct{}{};w.WriteHeader(http.StatusAccepted);return};http.NotFound(w,r)}));defer srv.Close();mgr.procs[1].apiPort=serverPort(t,srv)
	changed:=telemtInst(1,SecretEntry{Name:"alice",Secret:"0123456789abcdef0123456789abcdef"},SecretEntry{Name:"bob",Secret:"abcdef0123456789abcdef0123456789"});if err:=mgr.Ensure(changed);err!=nil{t.Fatalf("reload ensure: %v",err)}
	select{case<-reloaded:case<-time.After(time.Second):t.Fatal("expected Telemt reload")};if spawnCount(t,pidFile)!=1{t.Fatal("reload must keep process")};if mgr.procs[1].proc!=orig{t.Fatal("process changed during reload")}
	cfg,err:=os.ReadFile(configPathForID(1));if err!=nil{t.Fatal(err)};s:=string(cfg);if !strings.Contains(s,`"bob" = "abcdef0123456789abcdef0123456789"`){t.Fatalf("new user missing:\n%s",s)};if !strings.Contains(s,"[server.api]")||!strings.Contains(s,"Bearer "+token){t.Fatalf("API token must be reused:\n%s",s)}
	mgr.StopAll()
}

func TestEnsureReloadFallbackRestarts(t *testing.T){
	pidFile:=installFakeTelemt(t);mgr:=&Manager{procs:map[int]*managed{},swept:true};if err:=mgr.Ensure(telemtInst(2,SecretEntry{Name:"alice",Secret:"0123456789abcdef0123456789abcdef"}));err!=nil{t.Fatal(err)};waitSpawnCount(t,pidFile,1)
	srv:=httptest.NewServer(http.NotFoundHandler());defer srv.Close();mgr.procs[2].apiPort=serverPort(t,srv);if err:=mgr.Ensure(telemtInst(2,SecretEntry{Name:"carol",Secret:"abcdef0123456789abcdef0123456789"}));err!=nil{t.Fatal(err)};waitSpawnCount(t,pidFile,2);mgr.StopAll()
}
