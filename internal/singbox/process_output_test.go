package singbox

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestBoundedCommandOutputRetainsRecentDiagnostics(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("requires sh")
	}
	cmd := exec.Command(shell, "-c", "head -c 262144 /dev/zero; printf 'last diagnostic' >&2; exit 1")
	data, err := boundedCommandOutput(cmd)
	if err == nil || len(data) > processOutputLimit || !strings.HasSuffix(string(data), "last diagnostic") {
		t.Fatalf("output length=%d, error=%v, diagnostic retained=%v", len(data), err, strings.HasSuffix(string(data), "last diagnostic"))
	}
}

func TestBoundedCommandOutputDoesNotWaitForInheritedPipes(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("requires sh")
	}
	cmd := exec.Command(shell, "-c", "sleep 5 & exit 0")
	cmd.WaitDelay = 50 * time.Millisecond
	started := time.Now()
	_, err = boundedCommandOutput(cmd)
	if !errors.Is(err, exec.ErrWaitDelay) || time.Since(started) > 3*time.Second {
		t.Fatalf("inherited pipe was not bounded: elapsed=%v err=%v", time.Since(started), err)
	}
}

func TestProcessOutputSteadyWritesDoNotAllocate(t *testing.T) {
	output := &processOutput{}
	_, _ = output.Write([]byte(strings.Repeat("a", processOutputLimit)))
	line := []byte("INFO a short runtime log record\n")
	if allocations := testing.AllocsPerRun(100, func() { _, _ = output.Write(line) }); allocations != 0 {
		t.Fatalf("steady log writes allocate %v times per record", allocations)
	}
}

func TestProcessOutputRingPreservesChronologicalOrder(t *testing.T) {
	for _, size := range []int{1, 97, processOutputLimit - 1, processOutputLimit, processOutputLimit + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			output := &processOutput{}
			var all strings.Builder
			for i := 0; i < 7; i++ {
				part := strings.Repeat(string(rune('a'+i)), size)
				all.WriteString(part)
				n, err := output.Write([]byte(part))
				if err != nil || n != len(part) {
					t.Fatalf("Write=%d,%v", n, err)
				}
				expected := all.String()
				if len(expected) > processOutputLimit {
					expected = expected[len(expected)-processOutputLimit:]
				}
				if output.String() != expected {
					t.Fatalf("chronological output differs after write %d", i)
				}
			}
		})
	}
}
