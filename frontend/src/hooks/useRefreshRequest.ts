import { useCallback, useEffect, useRef, useState } from 'react';

// One live request per viewer; canceled responses cannot replace newer data.
export function useRefreshRequest<T>(
  open: boolean,
  load: (signal: AbortSignal) => Promise<T | undefined>,
  initial: T,
  intervalMs = 0,
) {
  const [data, setData] = useState<T>(initial);
  const [pending, setPending] = useState(false);
  const current = useRef<AbortController | null>(null);
  const run = useCallback(
    async (manual: boolean) => {
      if (!open || (!manual && (current.current || document.hidden))) return;
      current.current?.abort();
      const controller = new AbortController();
      current.current = controller;
      setPending(true);
      try {
        const value = await load(controller.signal);
        if (!controller.signal.aborted && value !== undefined) setData(() => value);
      } catch {
        // HttpUtil reports failures; keep the last good snapshot on transient errors.
      } finally {
        if (current.current === controller) {
          current.current = null;
          setPending(false);
        }
      }
    },
    [open, load],
  );
  const cancel = useCallback(() => {
    current.current?.abort();
    current.current = null;
  }, []);
  useEffect(() => {
    let active = true;
    queueMicrotask(() => {
      if (active && open) void run(true);
    });
    const timer =
      open && intervalMs > 0
        ? window.setInterval(() => {
            void run(false);
          }, intervalMs)
        : undefined;
    return () => {
      active = false;
      window.clearInterval(timer);
      cancel();
    };
  }, [open, run, intervalMs, cancel]);
  const refresh = useCallback(() => {
    void run(true);
  }, [run]);
  return { data, loading: open && pending, refresh };
}
