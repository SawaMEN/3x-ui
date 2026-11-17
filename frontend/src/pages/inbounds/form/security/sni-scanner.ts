import { HttpUtil } from '@/utils';
import type { RealityScanResult, SNIJobSnapshot } from '@/generated/types';

export type SNISnapshot = Omit<SNIJobSnapshot, 'results'> & { results: RealityScanResult[] };
function normalize(snapshot: SNIJobSnapshot): SNISnapshot {
  return {
    ...snapshot,
    results: snapshot.results.filter((row): row is RealityScanResult => row !== null),
  };
}
export type SNIProgress = (snapshot: SNISnapshot) => void;
export type SNIScanCallback = (
  targets?: string,
  mode?: string,
  signal?: AbortSignal,
  progress?: SNIProgress,
  recheck?: RealityScanResult[],
) => Promise<RealityScanResult[]>;

const base = '/panel/api/server/sni';
function nodeQuery(nodeId?: number | null) {
  return nodeId ? `?nodeId=${nodeId}` : '';
}
function pause(signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const abort = () => {
      clearTimeout(timer);
      signal?.removeEventListener('abort', abort);
      reject(new DOMException('Scan canceled', 'AbortError'));
    };
    const timer = setTimeout(() => {
      signal?.removeEventListener('abort', abort);
      resolve();
    }, 750);
    if (signal?.aborted) {
      abort();
      return;
    }
    signal?.addEventListener('abort', abort, { once: true });
  });
}
export async function runSNIScan(
  nodeId: number | null | undefined,
  purpose: string,
  targets?: string,
  mode = 'candidates',
  signal?: AbortSignal,
  progress?: SNIProgress,
  recheck?: RealityScanResult[],
): Promise<RealityScanResult[]> {
  const response = await HttpUtil.post<SNIJobSnapshot>(
    `${base}/jobs`,
    {
      mode,
      targets: targets ?? '',
      purpose,
      nodeId: nodeId ?? 0,
      ...(recheck ? { recheck: recheck.map(({ ip, host, port }) => ({ ip, host, port })) } : {}),
    },
    { silent: true, timeout: 15_000, signal, headers: { 'Content-Type': 'application/json' } },
  );
  if (!response.success || !response.obj?.id || !Array.isArray(response.obj.results)) {
    if (signal?.aborted) throw new DOMException('Scan canceled', 'AbortError');
    throw new Error(response.msg || 'SNI scan could not be started');
  }
  const url = `${base}/jobs/${encodeURIComponent(response.obj.id)}${nodeQuery(nodeId)}`;
  let current = normalize(response.obj);
  let cancelSent = false;
  const cancel = () => {
    if (!cancelSent) {
      cancelSent = true;
      void HttpUtil.delete(url, { silent: true, timeout: 10_000 });
    }
  };
  signal?.addEventListener('abort', cancel, { once: true });
  try {
    if (signal?.aborted) {
      cancel();
      throw new DOMException('Scan canceled', 'AbortError');
    }
    progress?.(current);
    while (
      current.state === 'queued' ||
      current.state === 'running' ||
      current.state === 'saving'
    ) {
      await pause(signal);
      const snapshot = await HttpUtil.get<SNIJobSnapshot>(url, undefined, {
        silent: true,
        timeout: 15_000,
        signal,
      });
      if (signal?.aborted) throw new DOMException('Scan canceled', 'AbortError');
      if (!snapshot.success || !snapshot.obj || !Array.isArray(snapshot.obj.results))
        throw new Error(snapshot.msg || 'SNI scan progress unavailable');
      current = normalize(snapshot.obj);
      progress?.(current);
    }
    if (current.state === 'failed') throw new Error(current.error || 'SNI scan failed');
    if (current.state === 'canceled') throw new DOMException('Scan canceled', 'AbortError');
    if (current.state !== 'done') throw new Error('Invalid SNI scan state');
    return current.results;
  } finally {
    signal?.removeEventListener('abort', cancel);
    if (current.state !== 'done' && current.state !== 'failed' && current.state !== 'canceled')
      cancel();
  }
}
export async function loadSNIHistory(
  nodeId?: number | null,
  signal?: AbortSignal,
): Promise<SNISnapshot[]> {
  const response = await HttpUtil.get<SNIJobSnapshot[]>(
    `${base}/history${nodeQuery(nodeId)}`,
    undefined,
    { silent: true, timeout: 15_000, signal },
  );
  if (!response.success || !Array.isArray(response.obj))
    throw new Error(response.msg || 'SNI scan history unavailable');
  return response.obj.map(normalize);
}
