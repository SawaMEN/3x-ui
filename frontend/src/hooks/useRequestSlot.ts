import { useCallback, useEffect, useRef } from 'react';

// A request slot owns one operation at a time. Aborting invalidates its result
// even if the underlying transport completes after cancellation.
export function useRequestSlot() {
  const current = useRef<AbortController | null>(null);
  const cancel = useCallback(() => {
    current.current?.abort();
    current.current = null;
  }, []);
  useEffect(() => cancel, [cancel]);

  const begin = useCallback(() => {
    cancel();
    const controller = new AbortController();
    current.current = controller;
    return controller;
  }, [cancel]);

  const finish = useCallback((controller: AbortController) => {
    if (current.current !== controller) return false;
    current.current = null;
    return !controller.signal.aborted;
  }, []);

  const isPending = useCallback(() => current.current !== null, []);
  return { begin, cancel, finish, isPending };
}
