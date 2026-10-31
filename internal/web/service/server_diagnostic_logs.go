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

// Keep the parsed instant next to its display record so sorting a large
// multi-source log result does not parse timestamps in every comparison.
type sortableDashboardLog struct {
    entry DashboardLog
    stamp time.Time
}

const dashboardLogLimit = 2000

var diagnosticSeverity = regexp.MustCompile(`(?i)\b(FATAL|CRITICAL|ERROR|ERR|WARNING|WARN|NOTICE|INFO|DEBUG|TRACE)\b`)
var diagnosticTimestamp = regexp.MustCompile(`\d{4}[-/]\d{2}[-/]\d{2}[ T]\d{2}:\d{2}:\d{2}(?:\.\d+)?`)
var diagnosticISOTime = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})`)

func diagnosticLevel(line string) string {
    // Raw runtime panics sometimes contain terms such as DEBUG or INFO in
    // their payload. Their leading panic marker must take precedence over
    // incidental severity words, otherwise ERROR-only filters hide crashes.
    rawPrefix := strings.ToLower(strings.TrimSpace(line))
    if strings.HasPrefix(rawPrefix, "panic:") || strings.HasPrefix(rawPrefix, "runtime: panic:") {
        return "ERROR"
    }
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
    // Prefer the earliest timestamp in the record. An ordinary panel log
    // may mention an RFC3339 date in its message; that later date must not
    // replace the actual "YYYY/MM/DD HH:MM:SS" log prefix.
    plain := diagnosticTimestamp.FindStringIndex(line)
    zoned := diagnosticISOTime.FindStringIndex(line)
    if zoned != nil && (plain == nil || zoned[0] <= plain[0]) {
        match := line[zoned[0]:zoned[1]]
        // Preserve explicit timezone offsets from journalctl/RFC3339 records.
        for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999-0700"} {
            if t, err := time.Parse(layout, match); err == nil { return t }
        }
    }
    if plain == nil { return time.Time{} }
    match := line[plain[0]:plain[1]]
    for _, layout := range []string{
        "2006/01/02 15:04:05.999999999",
        "2006-01-02 15:04:05.999999999",
        "2006-01-02T15:04:05.999999999",
    } {
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
// GetDashboardLogs keeps the original threshold filtering behavior for callers
// that don't request exact severity matching.
func (s *ServerService) GetDashboardLogs(count, level, source, query string) DashboardLogs {
    return s.GetDashboardLogsFiltered(count, level, source, query, "")
}

// GetDashboardLogsFiltered also accepts an exact, normalized severity selection.
// The "all" choice leaves the existing minimum-severity behavior unchanged.
func (s *ServerService) GetDashboardLogsFiltered(count, level, source, query, exactLevel string) DashboardLogs {
    exactLevel = strings.ToUpper(strings.TrimSpace(exactLevel))
    switch exactLevel {
    case "FATAL", "ERROR", "WARNING", "NOTICE", "INFO", "DEBUG", "UNKNOWN":
    default:
        exactLevel = ""
    }
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
    entries := make([]sortableDashboardLog, 0, limit)
    warnings := make([]string, 0, 3)
    appendLines := func(kind string, lines []string) {
        for _, raw := range lines {
            raw = strings.TrimRight(raw, "\r\n")
            if strings.TrimSpace(raw) == "" { continue }
            if search != "" && !strings.Contains(strings.ToLower(raw), search) { continue }
            severity := diagnosticLevel(raw)
            if exactLevel != "" {
                if severity != exactLevel { continue }
            } else if diagnosticRank(severity) > levelRank {
                continue
            }
            stamp := diagnosticTime(raw)
            item := DashboardLog{Source: kind, Level: severity, Raw: raw}
            if !stamp.IsZero() { item.Time = stamp.Format(time.RFC3339Nano) }
            entries = append(entries, sortableDashboardLog{entry: item, stamp: stamp})
        }
    }
    readFile := func(kind, path string) {
        if path == "" || path == "none" {
            warnings = append(warnings, kind+": журнал отключён в конфигурации")
            return
        }
        // Read only the newest scanLimit records. Previously this read up to
        // 8 MiB of every source before discarding all but 2000 lines,
        // multiplying memory and I/O when dozens of protocols are enabled.
        lines, err := tail.ReadTailLines(path, scanLimit, tail.DefaultTailBytes)
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
        if _, err := os.Stat(filepath.Join(config.GetLogFolder(), logger.LogFileName)); err != nil {
            if errors.Is(err, os.ErrNotExist) {
                warnings = append(warnings, "panel: файл журнала отсутствует (логирование может быть отключено)")
            } else {
                warnings = append(warnings, fmt.Sprintf("panel: не удалось проверить журнал: %v", err))
            }
        }
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
            seen := 0
            for _, dir := range dirs {
                if !dir.IsDir() { continue }
                kind := "protocol/" + dir.Name()
                path := filepath.Join(root, dir.Name(), "server.log")
                info, err := os.Stat(path)
                if errors.Is(err, os.ErrNotExist) { continue } // optional log
                if err != nil {
                    warnings = append(warnings, fmt.Sprintf("%s: не удалось проверить журнал: %v", kind, err))
                    continue
                }
                if !info.Mode().IsRegular() {
                    warnings = append(warnings, kind+": журнал не является обычным файлом")
                    continue
                }
                // Empty or inactive protocol directories must not exhaust the
                // journal cap and hide logs from active protocols sorted later.
                if seen >= 64 { warnings = append(warnings, "protocol: показаны первые 64 журнала"); break }
                seen++
                lines, err := tail.ReadTailLines(path, scanLimit, tail.DefaultTailBytes)
                if err != nil {
                    warnings = append(warnings, fmt.Sprintf("%s: не удалось прочитать журнал: %v", kind, err))
                    continue
                }
                appendLines(kind, lines)
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
        a, b := entries[i].stamp, entries[j].stamp
        if a.IsZero() { return false }
        if b.IsZero() { return true }
        return a.After(b)
    })
    if len(entries) > limit { entries = entries[:limit] }
    result := make([]DashboardLog, len(entries))
    for i := range entries { result[i] = entries[i].entry }
    return DashboardLogs{Entries: result, Warnings: warnings}
}
