package service

import (
	"encoding/gob"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestPersistSystemMetricsConcurrent(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := PersistSystemMetrics(); err != nil {
				t.Errorf("concurrent persistence failed: %v", err)
			}
		}()
	}
	wg.Wait()
	f, err := os.Open(systemMetricsStorePath())
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var data map[string]persistedSeries
	if err := gob.NewDecoder(f).Decode(&data); err != nil {
		t.Fatalf("persisted snapshot is unreadable: %v", err)
	}
}

func TestMetricMemoryFootprint(t *testing.T) {
	const metrics = 16

	retained := func(build func() any) uint64 {
		runtime.GC()
		runtime.GC()
		var m0 runtime.MemStats
		runtime.ReadMemStats(&m0)
		obj := build()
		runtime.GC()
		var m1 runtime.MemStats
		runtime.ReadMemStats(&m1)
		runtime.KeepAlive(obj)
		if m1.HeapAlloc < m0.HeapAlloc {
			return 0
		}
		return m1.HeapAlloc - m0.HeapAlloc
	}

	fill := func(buf []MetricSample) {
		for j := range buf {
			buf[j] = MetricSample{T: int64(j), V: float64(j)}
		}
	}

	oldFlat := retained(func() any {
		m := make(map[string][]MetricSample, metrics)
		for i := range metrics {
			buf := make([]MetricSample, 86400)
			fill(buf)
			m[fmt.Sprintf("m%d", i)] = buf
		}
		return m
	})

	newTiered := retained(func() any {
		h := newMetricHistory()
		for i := range metrics {
			s := newSeries()
			for _, tb := range s.tiers {
				buf := make([]MetricSample, tb.capacity)
				fill(buf)
				tb.samples = buf
			}
			h.series[fmt.Sprintf("m%d", i)] = s
		}
		return h
	})

	t.Logf("metric history footprint (16 system metrics, full):")
	t.Logf("  before (flat 48h@2s): %d KiB", oldFlat/1024)
	t.Logf("  after  (tiered 7d):   %d KiB", newTiered/1024)
	if newTiered >= oldFlat {
		t.Fatalf("expected tiered footprint smaller: old=%d new=%d", oldFlat, newTiered)
	}
}

func TestTierBufRollupAveragesClosedBuckets(t *testing.T) {
	tb := &tierBuf{resolution: 10, capacity: 100}
	tb.add(0, 2)
	tb.add(2, 4)
	tb.add(5, 6)
	tb.add(10, 10)

	if len(tb.samples) != 1 || tb.samples[0].T != 0 || tb.samples[0].V != 4 {
		t.Fatalf("expected one closed bucket {0,4}, got %+v", tb.samples)
	}

	got := tb.readSamples()
	if len(got) != 2 || got[1].T != 10 || got[1].V != 10 {
		t.Fatalf("expected open bucket {10,10} appended on read, got %+v", got)
	}
}

func TestTierBufRespectsCapacity(t *testing.T) {
	tb := &tierBuf{resolution: 1, capacity: 5}
	for i := range int64(20) {
		tb.add(i, float64(i))
	}
	if len(tb.samples) != 5 {
		t.Fatalf("expected closed buckets capped at 5, got %d", len(tb.samples))
	}
	closed := tb.readSamples()
	if last := closed[len(closed)-2]; last.T != 18 {
		t.Fatalf("expected last closed bucket T=18 (19 still open), got %d", last.T)
	}
}

func TestTierBufWrappedHistoryAndSnapshot(t *testing.T) {
	tb := &tierBuf{resolution: 1, capacity: 5}
	for i := range int64(23) {
		tb.add(i, float64(i))
	}
	got := tb.readSamples()
	for i, p := range got {
		want := int64(17 + i)
		if p.T != want || p.V != float64(want) {
			t.Fatalf("sample %d = %+v, want {%d,%d}", i, p, want, want)
		}
	}
	h := newMetricHistory()
	h.series["cpu"] = &series{tiers: []*tierBuf{tb}}
	closed := h.snapshot()["cpu"].Tiers[0].Samples
	for i, p := range closed {
		if p != got[i] {
			t.Fatalf("persisted sample %d = %+v, want %+v", i, p, got[i])
		}
	}
}

func TestTierBufFullWritesDoNotAllocate(t *testing.T) {
	tb := &tierBuf{resolution: 1, capacity: 5}
	var timestamp int64
	for range 10 {
		tb.add(timestamp, 1)
		timestamp++
	}
	allocs := testing.AllocsPerRun(1000, func() {
		for range 100 {
			tb.add(timestamp, 1)
			timestamp++
		}
	})
	if allocs != 0 {
		t.Fatalf("full ring writes allocated %g times", allocs)
	}
}

func TestTierBufIgnoresBackwardClockJump(t *testing.T) {
	tb := &tierBuf{resolution: 10, capacity: 5}
	tb.add(20, 2)
	tb.add(10, 100)
	tb.add(21, 4)
	tb.add(30, 8)
	got := tb.readSamples()
	if len(got) != 2 || got[0] != (MetricSample{T: 20, V: 3}) || got[1] != (MetricSample{T: 30, V: 8}) {
		t.Fatalf("backward clock jump corrupted buckets: %+v", got)
	}
}

func TestMetricRestoreOwnsBoundedSampleStorage(t *testing.T) {
	input := make([]MetricSample, 10000)
	for i := range input {
		input[i] = MetricSample{T: int64(i), V: float64(i)}
	}
	h := newMetricHistory()
	h.restore(map[string]persistedSeries{"cpu": {Tiers: []persistedTier{{Resolution: 2, Samples: input}}}})
	input[len(input)-1].V = -1
	tb := h.series["cpu"].tiers[0]
	if cap(tb.samples) != tb.capacity || len(tb.samples) != tb.capacity {
		t.Fatalf("restored tier retains oversized storage: len=%d cap=%d", len(tb.samples), cap(tb.samples))
	}
	if got := tb.readSamples(); got[len(got)-1].V != 9999 {
		t.Fatalf("restore aliases persisted samples: %+v", got[len(got)-1])
	}
}

func BenchmarkTierBufFullWrites(b *testing.B) {
	tb := &tierBuf{resolution: 1, capacity: 1800}
	var timestamp int64
	for range 2000 {
		tb.add(timestamp, 1)
		timestamp++
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		tb.add(timestamp, 1)
		timestamp++
	}
}

func TestSeriesPickTierBySpan(t *testing.T) {
	s := newSeries()
	cases := []struct {
		span int64
		res  int
	}{
		{120, 2},
		{3600, 2},
		{7200, 60},
		{172800, 60},
		{604800, 600},
		{9999999, 600},
	}
	for _, c := range cases {
		if got := s.pickTier(c.span); got.resolution != c.res {
			t.Errorf("span %d: expected resolution %d, got %d", c.span, c.res, got.resolution)
		}
	}
}

func TestAggregateFineRealtime(t *testing.T) {
	h := newMetricHistory()
	now := time.Now().Unix()
	for i := int64(59); i >= 0; i-- {
		h.append("cpu", time.Unix(now-i*2, 0), float64(100-i))
	}

	out := h.aggregate("cpu", 2, 60)
	if len(out) == 0 {
		t.Fatalf("expected non-empty realtime aggregate")
	}
	if _, ok := out[len(out)-1]["v"].(float64); !ok {
		t.Fatalf("expected float64 value, got %T", out[len(out)-1]["v"])
	}
}

func TestAggregateLongSpanUsesCoarseTier(t *testing.T) {
	h := newMetricHistory()
	now := time.Now().Unix()
	for i := range int64(200) {
		ts := now - (200-i)*600
		h.append("cpu", time.Unix(ts, 0), float64(i))
	}

	out := h.aggregate("cpu", 10080, 60)
	if len(out) == 0 {
		t.Fatalf("expected non-empty 7d aggregate from the archive tier")
	}
}

func TestSnapshotRestoreRoundTrip(t *testing.T) {
	h := newMetricHistory()
	now := time.Now().Unix()
	for i := range int64(10) {
		h.append("cpu", time.Unix(now-(9-i)*2, 0), float64(i))
	}

	h2 := newMetricHistory()
	h2.restore(h.snapshot())

	if out := h2.aggregate("cpu", 2, 60); len(out) == 0 {
		t.Fatalf("expected restored series to aggregate")
	}
}

func TestMetricSnapshotPreservesOpenBuckets(t *testing.T) {
	h := newMetricHistory()
	for _, v := range []float64{2, 4, 6} {
		h.append("cpu", time.Unix(1200, 0), v)
	}
	restored := newMetricHistory()
	restored.restore(h.snapshot())
	for i, tier := range h.series["cpu"].tiers {
		got := restored.series["cpu"].tiers[i].readSamples()
		if !reflect.DeepEqual(got, tier.readSamples()) {
			t.Fatalf("tier %d lost current bucket after restart: %v", tier.resolution, got)
		}
	}
	h.append("cpu", time.Unix(1201, 0), 8)
	restored.append("cpu", time.Unix(1201, 0), 8)
	for i, tier := range h.series["cpu"].tiers {
		if got := restored.series["cpu"].tiers[i].readSamples(); !reflect.DeepEqual(got, tier.readSamples()) {
			t.Fatalf("tier %d lost sample weights after restart: %v", tier.resolution, got)
		}
	}
}

func TestAggregateMissingMetricIsEmpty(t *testing.T) {
	h := newMetricHistory()
	if out := h.aggregate("nope", 2, 60); len(out) != 0 {
		t.Fatalf("expected empty result for unknown metric, got %d points", len(out))
	}
}
