import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { HttpUtil, TimeFormatter } from '@/utils';
import type { Status } from '@/models/status';

const OVERVIEW_WINDOW = 72;
const SEED_BUCKET_SECONDS = 2;

const SERIES_KEYS = [
  'cpu',
  'mem',
  'swap',
  'diskUsage',
  'netUp',
  'netDown',
  'tcpCount',
  'udpCount',
] as const;

export type OverviewSeriesKey = (typeof SERIES_KEYS)[number];

export interface OverviewHistory {
  series: Record<OverviewSeriesKey, number[]>;
  labels: string[];
  refreshHistory: () => void;
}

interface HistoryPoint {
  t: number;
  v: number;
}

interface HistoryWindow {
  series: Record<OverviewSeriesKey, number[]>;
  times: number[];
}

function emptySeries(): Record<OverviewSeriesKey, number[]> {
  return Object.fromEntries(SERIES_KEYS.map((key) => [key, [] as number[]])) as Record<
    OverviewSeriesKey,
    number[]
  >;
}

function emptyWindow(): HistoryWindow {
  return { series: emptySeries(), times: [] };
}

function sampleOf(status: Status): Record<OverviewSeriesKey, number> {
  return {
    cpu: status.cpu.percent,
    mem: status.mem.percent,
    swap: status.swap.percent,
    diskUsage: status.disk.percent,
    netUp: status.netIO.up,
    netDown: status.netIO.down,
    tcpCount: status.tcpCount,
    udpCount: status.udpCount,
  };
}

function tailWindow<T>(values: T[]): T[] {
  return values.slice(-OVERVIEW_WINDOW);
}

export function mean(values: number[]): number {
  if (values.length === 0) return 0;
  let total = 0;
  for (const v of values) total += v;
  return total / values.length;
}

export function peak(values: number[]): number {
  let max = 0;
  for (const v of values) if (v > max) max = v;
  return max;
}

/* The history endpoint accepts only backend-whitelisted bucket values. */
export function useOverviewHistory(
  status: Status,
  hasData: boolean,
  lowPower: boolean,
): OverviewHistory {
  const [trend, setTrend] = useState<HistoryWindow>(emptyWindow);
  const refreshGeneration = useRef(0);

  const refreshHistory = useCallback(() => {
    const generation = ++refreshGeneration.current;

    const seed = async () => {
      const responses = new Map<OverviewSeriesKey, HistoryPoint[]>();
      await Promise.all(
        SERIES_KEYS.map(async (key) => {
          const msg = await HttpUtil.get<HistoryPoint[]>(
            `/panel/api/server/history/${key}/${SEED_BUCKET_SECONDS}`,
            undefined,
            { silent: true },
          );
          if (msg?.success && Array.isArray(msg.obj)) responses.set(key, msg.obj);
        }),
      );
      if (generation !== refreshGeneration.current || responses.size === 0) return;

      let axis: HistoryPoint[] = [];
      for (const points of responses.values()) {
        if (points.length > axis.length) axis = points;
      }
      axis = tailWindow(axis);
      if (axis.length === 0) return;

      const seedTimes = axis.map((p) => Number(p.t) || 0);
      const seedSeries = emptySeries();
      for (const key of SERIES_KEYS) {
        const byTs = new Map<number, number>();
        for (const p of responses.get(key) ?? []) {
          byTs.set(Number(p.t) || 0, Number(p.v) || 0);
        }
        seedSeries[key] = seedTimes.map((ts) => byTs.get(ts) ?? 0);
      }

      setTrend((prev) => {
        if (generation !== refreshGeneration.current) return prev;
        const merged = emptyWindow();
        merged.times = tailWindow(seedTimes.concat(prev.times));
        for (const key of SERIES_KEYS) {
          merged.series[key] = tailWindow(seedSeries[key].concat(prev.series[key]));
        }
        return merged;
      });
    };

    void seed().catch(() => undefined);
  }, []);

  useEffect(() => {
    refreshHistory();
  }, [refreshHistory, lowPower]);

  const lastSampleRef = useRef<{ status: Status | null; at: number }>({
    status: null,
    at: 0,
  });

  useEffect(() => {
    if (lowPower || !hasData || lastSampleRef.current.status === status) return;

    const now = Date.now();
    // The status poll can occasionally fire twice in the same task. Keep one
    // chart sample per refreshed status object so the history window stays
    // stable and does not grow from duplicate renders.
    if (now - lastSampleRef.current.at < 100) return;

    lastSampleRef.current = { status, at: now };
    // Build the next window once. Keeping the state update as a single
    // immutable operation avoids eight separate array updates per status tick.
    const point = sampleOf(status);
    setTrend((prev) => {
      const next = emptyWindow();
      next.times = tailWindow(prev.times.concat(Math.floor(now / 1000)));
      for (const key of SERIES_KEYS) {
        next.series[key] = tailWindow(prev.series[key].concat(point[key]));
      }
      return next;
    });
  }, [status, hasData, lowPower]);

  const labels = useMemo(() => trend.times.map(TimeFormatter.formatClock), [trend.times]);

  return useMemo(
    () => ({ series: trend.series, labels, refreshHistory }),
    [trend.series, labels, refreshHistory],
  );
}
