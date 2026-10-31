package service

import (
    "bytes"
    "context"
    "errors"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "regexp"
    "runtime"
    "sort"
    "strconv"
    "strings"
    "time"

    "github.com/SawaMEN/3x-ui/v3/internal/config"
    "github.com/SawaMEN/3x-ui/v3/internal/logger"
    "github.com/SawaMEN/3x-ui/v3/internal/util/tail"
    "github.com/SawaMEN/3x-ui/v3/internal/xray"
)

// DashboardLog is a lossless, source-tagged log record. Raw is preserved for
// copying and diagnostics: parsing must never hide error details or stack traces.
type DashboardLog struct {
    Source string `json:"source"`
    Level string `json:"level"`
    Time string `json:"time,omitempty"`
    Raw string `json:"raw"`
}

type DashboardLogs struct {
    Entries []DashboardLog `json:"entries"`
    Warnings []string `json:"warnings"`
}

const dashboardLogLimit = 2000

var diagnosticSeverity = regexp.MustCompile(`(?i)\b(FATAL|CRITICAL|ERROR|ERR|WARNING|WARN|NOTICE|INFO|DEBUG|TRACE)\b`)
var diagnosticTimestamp = regexp.MustCompile(`\d{4}[-/]\d{2}[-/]\d{2}[ T]\d{2}:\d{2}:\d{2}`)

func diagnosticLevel(line string) string {
    // Restrict detection to the log prefix, not a URL or message discussing an error.
    prefix := line
    if len(prefix) > 115 { prefix = prefix[:115] }
    match := diagnosticSeverity.FindStringSubmatch(prefix)
    if len(match) < 2 {
        // Many third-party processes and systemd units omit an explicit
        // severity token. Recognize common failure messages so ERROR-only
        // views do not silently discard TLS handshake failures and panics.
        lower := strings.ToLower(line)
        for _, symptom := range []string{
            "panic:", "fatal:", "failed to ", "failure:", "permission denied",
            "connection refused", "bad certificate", "tls handshake:",
            "invalid sudoku hint", "unexpected eof", "segmentation fault",
        } {
            if strings.Contains(lower, symptom) { return "ERROR" }
        }
        for _, symptom := range []string{"retrying", "not ready", "timed out", "timeout"} {
            if strings.Contains(lower, symptom) { return "WARNING" }
        }
        return "UNKNOWN"
    }
    switch strings.ToUpper(match[1]) {
    case "FATAL", "CRITICAL": return "FATAL"
    case "ERR", "ERROR": return "ERROR"
    case "WARNING", "WARN": return "WARNING"
    case "NOTICE": return "NOTICE"
    case "INFO": return "INFO"
    default: return "DEBUG"
    }
}

func diagnosticTime(line string) time.Time {
    if len(line) >= 24 && strings.HasPrefix(line[4:], "-") {
        // journalctl -o short-iso: 2026-11-01T12:34:56+0200
        if t, err := time.Parse("2006-01-02T15:04:05-0700", line[:24]); err == nil {
            return t
        }
    }
    match := diagnosticTimestamp.FindString(line)
    if match == "" { return time.Time{} }
    for _, layout := range []string{"2006/01/02 15:04:05", "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
        if t, err := time.ParseInLocation(layout, match, time.Local); err == nil { return t }
    }
    return time.Time{}
}

func diagnosticRank(level string) int {
    switch level {
    case "FATAL": return 0
    case "ERROR": return 1
    case "WARNING": return 2
    case "NOTICE": return 3
    case "INFO": return 4
    case "DEBUG": return 5
    default: return 4 // Unknown severity must remain visible at INFO.
    }
}

func dashboardLogCount(raw string) int {
    n, err := strconv.Atoi(raw)
    if err != nil || n < 1 { return 100 }
    if n > dashboardLogLimit { return dashboardLogLimit }
    return n
}

// GetDashboardLogs aggregates *existing* durable logs. It does not enable
// debug/access logging automatically (which can expose user traffic or secrets).
// Sources are explicitly identified; unavailable files are reported, never
// silently mistaken for an empty or healthy runtime.
func (s *ServerService) GetDashboardLogs(count, level, source, query string) DashboardLogs {
    limit := dashboardLogCount(count)
    source = strings.ToLower(strings.TrimSpace(source))
    if source != "panel" && source != "core" && source != "access" && source != "journal" && source != "protocol" && source != "naive" && source != "update" && source != "ip-limit" && source != "all" {
        source = "all"
    }
    threshold := strings.ToLower(strings.TrimSpace(level))
    if threshold == "err" { threshold = "error" }
    switch threshold {
    case "debug", "info", "notice", "warning", "error":
    default: threshold = "debug"
    }
    levelRank := diagnosticRank(strings.ToUpper(threshold))
    search := strings.ToLower(strings.TrimSpace(query))
    // Fetch more than the final count for each source so filtering does not
    // hide an older matching error behind recent INFO events.
    scanLimit := dashboardLogLimit
    entries := make([]DashboardLog, 0, limit)
    warnings := make([]string, 0, 3)
    appendLines := func(kind string, lines []string) {
        for _, raw := range lines {
            raw = strings.TrimRight(raw, "\r\n")
            if strings.TrimSpace(raw) == "" { continue }
            if search != "" && !strings.Contains(strings.ToLower(raw), search) { continue }
            severity := diagnosticLevel(raw)
            if diagnosticRank(severity) > levelRank { continue }
            stamp := diagnosticTime(raw)
            item := DashboardLog{Source: kind, Level: severity, Raw: raw}
            if !stamp.IsZero() { item.Time = stamp.Format(time.RFC3339) }
            entries = append(entries, item)
        }
    }
    readFile := func(kind, path string) {
        if path == "" || path == "none" {
            warnings = append(warnings, kind+": журнал отключён в конфигурации")
            return
        }
        lines, err := tail.ReadTailLines(path, 0, tail.DefaultTailBytes)
        if err != nil {
            if errors.Is(err, os.ErrNotExist) {
                warnings = append(warnings, kind+": файл журнала отсутствует (логирование может быть отключено)")
            } else {
                warnings = append(warnings, fmt.Sprintf("%s: не удалось прочитать журнал: %v", kind, err))
            }
            return
        }
        if len(lines) > scanLimit { lines = lines[:scanLimit] }
        appendLines(kind, lines)
    }

    if source == "all" || source == "panel" {
        // Panel messages are not restricted to the in-memory buffer's 4096
        // entries and remain available across panel restarts.
        lines := logger.GetFileLogs(scanLimit, "debug")
        // The file backend can have a stricter threshold than the in-memory
        // ring (e.g. INFO on disk but DEBUG messages in memory). Merge both
        // without duplicating records already persisted in the file.
        persisted := make(map[string]int, len(lines))
        for _, line := range lines { persisted[line]++ }
        for _, line := range logger.GetLogs(scanLimit, "debug") {
            if persisted[line] > 0 {
                persisted[line]--
                continue
            }
            lines = append(lines, line)
        }
        appendLines("panel", lines)
    }
    if source == "all" || source == "core" || source == "access" {
        coreType, err := (&SettingService{}).GetCoreType()
        if err != nil {
            warnings = append(warnings, "core: не удалось определить выбранное ядро: "+err.Error())
        } else if IsNativeCore(coreType) {
            if source == "all" || source == "core" {
                readFile("core", NativeCore(coreType).logPath())
            }
            if source == "access" {
                warnings = append(warnings, "access: подключения sing-box находятся в журнале ядра; выберите «Ядро»")
            }
        } else {
            if source == "all" || source == "core" {
                path, err := xray.GetErrorLogPath()
                if err != nil { warnings = append(warnings, "core: "+err.Error()) } else { readFile("core", path) }
            }
            if source == "all" || source == "access" {
                path, err := xray.GetAccessLogPath()
                if err != nil { warnings = append(warnings, "access: "+err.Error()) } else { readFile("access", path) }
            }
        }
    }
    if source == "all" || source == "update" {
        readFile("update", "/var/log/x-ui/update.log")
    }
    if source == "all" || source == "naive" {
        readFile("naive", filepath.Join(config.GetBinFolderPath(), "caddy-naive.log"))
    }
    if source == "all" || source == "ip-limit" {
        readFile("ip-limit", xray.GetIPLimitLogPath())
        readFile("ip-ban", xray.GetIPLimitBannedLogPath())
    }
    if source == "all" || source == "protocol" {
        root := filepath.Join(config.GetBinFolderPath(), "protocol-servers")
        dirs, err := os.ReadDir(root)
        if err == nil {
            for idx, dir := range dirs {
                if idx >= 64 { warnings = append(warnings, "protocol: показаны первые 64 каталога"); break }
                if !dir.IsDir() { continue }
                path := filepath.Join(root, dir.Name(), "server.log")
                lines, err := tail.ReadTailLines(path, scanLimit, tail.DefaultTailBytes)
                if err == nil { appendLines("protocol/"+dir.Name(), lines) }
            }
        } else if !errors.Is(err, os.ErrNotExist) {
            warnings = append(warnings, "protocol: "+err.Error())
        }
    }
    if source == "all" || source == "journal" {
        if runtime.GOOS == "windows" {
            warnings = append(warnings, "journal: systemd недоступен в Windows")
        } else {
            ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
            defer cancel()
            cmd := exec.CommandContext(ctx, "journalctl", "--no-pager", "--output=short-iso",
                "--lines="+strconv.Itoa(scanLimit), "-p", "debug",
                "-u", "x-ui.service", "-u", "telemt.service", "-u", "x-ui-oxidns.service",
                "-u", "x-ui-vk-turn-proxy.service")
            output, err := cmd.Output()
            if err != nil {
                if errors.Is(ctx.Err(), context.DeadlineExceeded) {
                    warnings = append(warnings, "journal: превышено время ожидания journalctl")
                } else {
                    warnings = append(warnings, "journal: journalctl недоступен или чтение запрещено")
                }
            } else {
                // journalctl writes oldest first. The UI contract is newest first.
                lines := strings.Split(strings.TrimRight(string(bytes.TrimSpace(output)), "\n"), "\n")
                for i,j := 0,len(lines)-1; i<j; i,j=i+1,j-1 { lines[i],lines[j]=lines[j],lines[i] }
                appendLines("journal", lines)
            }
        }
    }

    // Sources have independently timestamped records. Keep most recent
    // entries first while retaining relative order for equal/unknown stamps.
    sort.SliceStable(entries, func(i, j int) bool {
        a, ea := time.Parse(time.RFC3339, entries[i].Time)
        b, eb := time.Parse(time.RFC3339, entries[j].Time)
        if ea != nil { return false }
        if eb != nil { return true }
        return a.After(b)
    })
    if len(entries) > limit { entries = entries[:limit] }
    return DashboardLogs{Entries: entries, Warnings: warnings}
}
