import { useEffect, useState } from 'react';
import { HttpUtil } from '@/utils';

// Status reads never install or download a component, including on a remote node.
export function useOpenFluxInstalled(open: boolean, nodeId = 0): boolean {
  const [status, setStatus] = useState<{ nodeId: number; installed: boolean } | null>(null);
  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setStatus(null);
    const suffix = nodeId > 0 ? `?nodeId=${nodeId}` : '';
    void HttpUtil.get<{ installed?: boolean }>(
      `/panel/api/server/openflux/status${suffix}`, undefined, { silent: true },
    ).then((result) => {
      if (!cancelled) setStatus({ nodeId, installed: result.success === true && result.obj?.installed === true });
    }).catch(() => {
      if (!cancelled) setStatus({ nodeId, installed: false });
    });
    return () => { cancelled = true; };
  }, [open, nodeId]);
  return open && status?.nodeId === nodeId && status.installed;
}
