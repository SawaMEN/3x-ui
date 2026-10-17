package dnstt

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagedServiceLifecycle(t *testing.T) {
	if os.Getenv("DNSTT_SYSTEMD_TEST") != "1" {
		t.Skip("requires disposable Linux runner booted with systemd")
	}
	if os.Geteuid() != 0 {
		t.Fatal("systemd test must run as root")
	}
	binary := os.Getenv("DNSTT_SERVER_BINARY")
	if !filepath.IsAbs(binary) {
		t.Fatal("required verified binary")
	}
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		t.Fatal("runner does not use systemd")
	}
	for _, path := range []string{Root,unitPath} {
		if _,err:=os.Lstat(path);err==nil {
			t.Fatalf("refusing to modify existing DNSTT setup %s",path)
		}else if !errors.Is(err,os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	ctx,cancel:=context.WithTimeout(context.Background(),70*time.Second)
	defer cancel()
	t.Cleanup(func(){
		c,cancel:=context.WithTimeout(context.Background(),15*time.Second)
		defer cancel()
		if err:=Remove(c);err!=nil{t.Logf("cleanup failed: %v",err)}
	})
	payload,err:=os.ReadFile(binary)
	if err!=nil{t.Fatal(err)}
	if err:=fileWrite(binaryPath,payload,0755);err!=nil{t.Fatal(err)}
	if err:=fileWrite(versionPath,[]byte(Version+"\n"),0600);err!=nil{t.Fatal(err)}
	ssh,err:=net.Listen("tcp","127.0.0.1:0")
	if err!=nil{t.Fatal(err)}
	defer ssh.Close()
	target:=ssh.Addr().(*net.TCPAddr).Port
	udp,err:=net.ListenPacket("udp","127.0.0.1:0")
	if err!=nil{t.Fatal(err)}
	listenPort:=udp.LocalAddr().(*net.UDPAddr).Port
	udp.Close()
	cfg:=Config{Domain:"t.example.com",ServerIP:"203.0.113.2",ListenPort:listenPort,TargetPort:target}
	if err:=Setup(ctx,cfg);err!=nil{t.Fatalf("DNSTT systemd setup: %v",err)}
	status,err:=StatusNow(ctx)
	if err!=nil||!status.Installed||!status.Enabled||!status.Running||len(status.PublicKey)!=64 {
		t.Fatalf("DNSTT was not successfully started and keyed: %+v %v",status,err)
	}
	if err:=Disable(ctx);err!=nil{t.Fatalf("DNSTT disable failed: %v",err)}
	off,err:=StatusNow(ctx)
	if err!=nil||off.Enabled||off.Running {t.Fatalf("DNSTT service still active: %+v %v",off,err)}
	if err:=Setup(ctx,cfg);err!=nil{t.Fatalf("DNSTT reenabling failed: %v",err)}
	on,err:=StatusNow(ctx)
	if err!=nil||!on.Running||on.PublicKey!=status.PublicKey {
		t.Fatalf("DNSTT key changed on restart: %+v %v",on,err)
	}
	if err:=Remove(ctx);err!=nil{t.Fatalf("DNSTT removal failed: %v",err)}
	for _, p := range []string{Root,unitPath} {
		if _,err:=os.Lstat(p);!errors.Is(err,os.ErrNotExist) {
			t.Errorf("DNSTT left a managed file: %s (%v)",p,err)
		}
	}
	uninstalled,err:=StatusNow(ctx)
	if err!=nil||uninstalled.Installed||uninstalled.Running||uninstalled.PublicKey!="" {
		t.Fatalf("DNSTT was not fully removed: %+v %v",uninstalled,err)
	}
	if err:=Remove(ctx);err!=nil{t.Errorf("DNSTT second removal must be safe: %v",err)}
}
