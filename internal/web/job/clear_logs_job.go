package job

import (
	"io"
	"os"
	"path/filepath"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

const defaultMaxXrayLogBytes int64 = 64 << 20
var maxXrayLogBytes = defaultMaxXrayLogBytes
type ClearLogsJob struct{}
type PruneXrayLogsJob struct{}
func NewClearLogsJob()*ClearLogsJob{return new(ClearLogsJob)}
func NewPruneXrayLogsJob()*PruneXrayLogsJob{return new(PruneXrayLogsJob)}
func ensureFileExists(path string)error{dir:=filepath.Dir(path);if err:=os.MkdirAll(dir,0o755);err!=nil{return err};f,err:=os.OpenFile(path,os.O_CREATE|os.O_RDWR,0o644);if err!=nil{return err};f.Close();return nil}
func(j *ClearLogsJob)Run(){
	logFiles:=[]string{xray.GetIPLimitLogPath(),xray.GetIPLimitBannedLogPath()};prev:=[]string{xray.GetIPLimitBannedPrevLogPath()}
	for _,path:=range append(logFiles,prev...){if err:=ensureFileExists(path);err!=nil{logger.Warning("Failed to ensure log file exists:",path,"-",err)}}
	for i:=range len(logFiles){if i>0{dst,err:=os.OpenFile(prev[i-1],os.O_CREATE|os.O_TRUNC|os.O_WRONLY,0o644);if err!=nil{logger.Warning("Failed to open previous log file for writing:",prev[i-1],"-",err);continue};src,err:=os.OpenFile(logFiles[i],os.O_RDONLY,0o644);if err!=nil{logger.Warning("Failed to open current log file for reading:",logFiles[i],"-",err);dst.Close();continue};if _,err=io.Copy(dst,src);err!=nil{logger.Warning("Failed to copy log file:",err)};src.Close();dst.Close()};if err:=os.Truncate(logFiles[i],0);err!=nil{logger.Warning("Failed to truncate log file:",logFiles[i],"-",err)}}
	wipeXrayLogs();checkpointSQLiteWAL()
}
func(j *PruneXrayLogsJob)Run(){truncateXrayLog(xray.GetAccessLogPath,maxXrayLogBytes);truncateXrayLog(xray.GetErrorLogPath,maxXrayLogBytes)}
func checkpointSQLiteWAL(){if database.IsPostgres(){return};db:=database.GetDB();if db==nil{return};if err:=db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error;err!=nil{logger.Warning("SQLite WAL checkpoint failed:",err)}}
func wipeXrayLogs(){truncateXrayLog(xray.GetAccessLogPath,0);truncateXrayLog(xray.GetErrorLogPath,0)}
func truncateXrayLog(pathFn func()(string,error),maxBytes int64){logPath,err:=pathFn();if err!=nil||disabledLogPath(logPath){return};if maxBytes>0{info,err:=os.Stat(logPath);if err!=nil{if !os.IsNotExist(err){logger.Warning("Failed to stat Xray log:",logPath,"-",err)};return};if info.Size()<=maxBytes{return}};if err:=os.Truncate(logPath,0);err!=nil&&!os.IsNotExist(err){logger.Warning("Failed to truncate Xray log:",logPath,"-",err)}}
func disabledLogPath(path string)bool{return path==""||path=="none"}
