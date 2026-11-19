package zapret2

// Official-release component installer. This package deliberately does NOT run
// upstream install_easy.sh: 3x-ui owns its NFQUEUE rules and service lifecycle.
import (
 "archive/tar"
 "compress/gzip"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "sync"
 "time"

 "github.com/SawaMEN/3x-ui/v3/internal/config"
)

const releasesAPI = "https://api.github.com/repos/bol-van/zapret2/releases/latest"
const maxArchiveBytes int64 = 128 << 20

var installMu sync.Mutex
var client = &http.Client{Timeout: 90 * time.Second}

type Status struct {
 Installed bool `json:"installed"`
 Supported bool `json:"supported"`
 UnsupportedReason string `json:"unsupportedReason,omitempty"`
 Version string `json:"version"`
 LatestVersion string `json:"latestVersion"`
 UpdateAvailable bool `json:"updateAvailable"`
 Binary string `json:"binary"`
 Source string `json:"source"`
}

type release struct {
 Tag string `json:"tag_name"`
 Assets []struct {
  Name string `json:"name"`
  URL string `json:"browser_download_url"`
  Digest string `json:"digest"`
 } `json:"assets"`
}

func Directory() string { return filepath.Join(config.GetBinFolderPath(), "zapret2") }
func BinaryPath() string { return filepath.Join(Directory(), "bin", "nfqws2") }
func Installed() bool {
 st, err := os.Stat(BinaryPath())
 if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0111 == 0 { return false }
 _, err = os.Stat(filepath.Join(Directory(), "lua", "zapret-lib.lua"))
 return err == nil
}
func supported() bool { return runtime.GOOS == "linux" && (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64") }

func fetchRelease(ctx context.Context) (release,error) {
 req,err := http.NewRequestWithContext(ctx,http.MethodGet,releasesAPI,nil)
 if err != nil { return release{},err }
 req.Header.Set("Accept","application/vnd.github+json")
 req.Header.Set("User-Agent","3x-ui-zapret2-installer")
 resp,err := client.Do(req)
 if err != nil { return release{},err }
 defer resp.Body.Close()
 if resp.StatusCode != http.StatusOK { return release{},fmt.Errorf("official zapret2 release API returned %s",resp.Status) }
 var rel release
 if err := json.NewDecoder(io.LimitReader(resp.Body,2<<20)).Decode(&rel); err != nil { return release{},err }
 if !strings.HasPrefix(rel.Tag,"v") || len(rel.Tag)>64 || strings.ContainsAny(rel.Tag,"/\\") { return release{},fmt.Errorf("invalid official zapret2 release tag") }
 return rel,nil
}

func GetStatus(ctx context.Context) (Status,error) {
 status := Status{Installed: Installed(),Supported:supported(),Binary:BinaryPath(),Source:"bol-van/zapret2"}
 if !status.Supported { status.UnsupportedReason = "zapret2 NFQUEUE requires Linux amd64/arm64" }
 if data,err := os.ReadFile(filepath.Join(Directory(),"version")); err==nil { status.Version=strings.TrimSpace(string(data)) }
 lookupCtx,cancel:=context.WithTimeout(ctx,7*time.Second)
 defer cancel()
 rel,err := fetchRelease(lookupCtx)
 if err == nil {
  status.LatestVersion = rel.Tag
  status.UpdateAvailable = status.Installed && status.Version!="" && status.Version != rel.Tag
 }
 // A GitHub outage must not hide an already installed working component.
 return status,nil
}

func Update(ctx context.Context) (Status,error) {
 if Running() {return Status{},fmt.Errorf("stop the VPN core using zapret2 before updating the component")}
 installMu.Lock()
 defer installMu.Unlock()
 if !supported() { return Status{},fmt.Errorf("zapret2 requires Linux amd64 or arm64") }
 rel,err := fetchRelease(ctx)
 if err != nil { return Status{},err }
 archiveName := "zapret2-"+rel.Tag+".tar.gz"
 var assetURL,digest string
 for _,a := range rel.Assets {
  if a.Name == archiveName { assetURL=a.URL; digest=a.Digest; break }
 }
 if assetURL=="" || !strings.HasPrefix(digest,"sha256:") { return Status{},fmt.Errorf("official release missing archive or SHA-256 digest") }
 if !strings.HasPrefix(assetURL,"https://github.com/bol-van/zapret2/releases/download/") { return Status{},fmt.Errorf("unexpected upstream archive URL") }
 if err:=os.MkdirAll(filepath.Dir(Directory()),0755);err!=nil{return Status{},err}
 stage,err := os.MkdirTemp(filepath.Dir(Directory()),".zapret2-stage-*")
 if err != nil { return Status{},err }
 defer os.RemoveAll(stage)
 req,err := http.NewRequestWithContext(ctx,http.MethodGet,assetURL,nil)
 if err != nil { return Status{},err }
 req.Header.Set("User-Agent","3x-ui-zapret2-installer")
 resp,err := client.Do(req)
 if err != nil { return Status{},err }
 if resp.StatusCode != http.StatusOK { resp.Body.Close(); return Status{},fmt.Errorf("download official zapret2: %s",resp.Status) }
 archive,err := os.CreateTemp(stage,"archive-*.tar.gz")
 if err != nil { resp.Body.Close();return Status{},err }
 hasher := sha256.New()
 n,copyErr := io.Copy(io.MultiWriter(archive,hasher),io.LimitReader(resp.Body,maxArchiveBytes+1))
 closeErr := errors.Join(resp.Body.Close(),archive.Close())
 if copyErr != nil || closeErr != nil { return Status{},errors.Join(copyErr,closeErr) }
 if n>maxArchiveBytes { return Status{},fmt.Errorf("zapret2 release archive too large") }
 if actual:=hex.EncodeToString(hasher.Sum(nil)); !strings.EqualFold(actual,strings.TrimPrefix(digest,"sha256:")) {
  return Status{},fmt.Errorf("official zapret2 archive SHA-256 mismatch")
 }
 extracted:=filepath.Join(stage,"extract")
 if err:=os.MkdirAll(extracted,0755);err!=nil{return Status{},err}
 if err:=unpackArchive(archive.Name(),extracted);err!=nil{return Status{},err}
 root,err:=findDistributionRoot(extracted)
 if err!=nil{return Status{},err}
 binary,err:=findBinary(root)
 if err!=nil{return Status{},err}
 target:=filepath.Join(root,"bin","nfqws2")
 if err:=os.MkdirAll(filepath.Dir(target),0755);err!=nil{return Status{},err}
 if err:=copyBinary(binary,target);err!=nil{return Status{},err}
 // The official blockcheck scripts use traditional relative binary paths.
 // Recreate the links that upstream install_bin.sh normally installs, without
 // running the upstream installer or altering the host firewall.
 for _,spec:=range []struct{name,dir string}{
  {"nfqws2","nfq2"},{"mdig","mdig"},{"ip2net","ip2net"},
 } {
  source,err:=findExecutable(root,spec.name)
  if err!=nil {return Status{},err}
  dest:=filepath.Join(root,spec.dir,spec.name)
  if err:=copyBinary(source,dest);err!=nil{return Status{},err}
 }
 if err:=os.WriteFile(filepath.Join(root,"version"),[]byte(rel.Tag+"\n"),0644);err!=nil{return Status{},err}
 destination:=Directory()
 backup:=filepath.Join(stage,"previous")
 oldPresent:=false
 if _,err:=os.Stat(destination);err==nil {
  oldPresent=true
  if err:=os.Rename(destination,backup);err!=nil{return Status{},err}
 } else if !os.IsNotExist(err) {return Status{},err}
 if err:=os.Rename(root,destination);err!=nil {
  if oldPresent { _=os.Rename(backup,destination) }
  return Status{},err
 }
 return Status{Installed:true,Supported:true,Version:rel.Tag,LatestVersion:rel.Tag,Binary:BinaryPath(),Source:"bol-van/zapret2"},nil
}

func Remove() error {
 if Running(){return fmt.Errorf("cannot remove zapret2 while its NFQUEUE process is running")}
 installMu.Lock()
 defer installMu.Unlock()
 return os.RemoveAll(Directory())
}

func unpackArchive(path,destination string) error {
 file,err:=os.Open(path);if err!=nil{return err};defer file.Close()
 gz,err:=gzip.NewReader(file);if err!=nil{return err};defer gz.Close()
 tr:=tar.NewReader(gz)
 for {
  hdr,err:=tr.Next()
  if errors.Is(err,io.EOF){break}
  if err!=nil{return err}
  clean:=filepath.Clean(hdr.Name)
  if filepath.IsAbs(clean)||clean==".."||strings.HasPrefix(clean,".."+string(os.PathSeparator)){return fmt.Errorf("unsafe archive path %q",hdr.Name)}
  full:=filepath.Join(destination,clean)
  switch hdr.Typeflag {
  case tar.TypeDir:
   if err:=os.MkdirAll(full,0755);err!=nil{return err}
  case tar.TypeReg,tar.TypeRegA:
   if hdr.Size<0 || hdr.Size>maxArchiveBytes {return fmt.Errorf("invalid zapret2 member size")}
   if err:=os.MkdirAll(filepath.Dir(full),0755);err!=nil{return err}
   mode:=os.FileMode(hdr.Mode)&0755
   if mode==0 {mode=0644}
   f,err:=os.OpenFile(full,os.O_CREATE|os.O_EXCL|os.O_WRONLY,mode)
   if err!=nil{return err}
   _,copyErr:=io.CopyN(f,tr,hdr.Size)
   if err:=errors.Join(copyErr,f.Close());err!=nil{return err}
  // Do not import links, device nodes or special files from upstream archives.
  default:
  }
 }
 return nil
}

func findDistributionRoot(root string) (string,error) {
 var found string
 _=filepath.WalkDir(root,func(path string,d os.DirEntry,err error) error {
  if err!=nil{return err}
  if d.IsDir(){return nil}
  if d.Name()=="zapret-lib.lua" && filepath.Base(filepath.Dir(path))=="lua" {found=filepath.Dir(filepath.Dir(path))}
  return nil
 })
 if found=="" {return "",fmt.Errorf("official archive missing zapret-lib.lua")}
 if _,err:=os.Stat(filepath.Join(found,"blockcheck2.sh"));err!=nil{return "",fmt.Errorf("official archive missing blockcheck2.sh")}
 return found,nil
}

func findBinary(root string) (string,error) { return findExecutable(root,"nfqws2") }

func findExecutable(root,name string) (string,error) {
 arch:="x86_64"
 if runtime.GOARCH=="arm64" {arch="arm64"}
 var found string
 _=filepath.WalkDir(root,func(path string,d os.DirEntry,err error) error {
  if err!=nil{return err}
  if !d.IsDir() && d.Name()==name && strings.Contains(path,"binaries") && strings.Contains(path,"linux-"+arch) {found=path}
  return nil
 })
 if found=="" {return "",fmt.Errorf("official zapret2 release lacks Linux %s %s binary",arch,name)}
 return found,nil
}
func copyBinary(from,to string) error {
 src,err:=os.Open(from);if err!=nil{return err};defer src.Close()
 out,err:=os.OpenFile(to,os.O_CREATE|os.O_TRUNC|os.O_WRONLY,0755);if err!=nil{return err}
 _,copyErr:=io.Copy(out,src)
 return errors.Join(copyErr,out.Close())
}
