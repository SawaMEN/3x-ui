import { useEffect, useState } from 'react';
import { HttpUtil } from '@/utils';
import CoreSelection from './CoreSelection';

interface ApiMsg<T> {
  success?: boolean;
  obj?: T;
}

export default function ProxyCoreTab({ active }: { active: boolean }) {
  const [installed, setInstalled] = useState<boolean | null>(null);
  const [running, setRunning] = useState(false);
  useEffect(() => {
    if (!active) return;
    let cancelled = false;
    let inFlight = false;
    let attempts = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const refresh = async () => {
      if (cancelled || inFlight) return;
      if (timer) clearTimeout(timer);
      inFlight = true;
      attempts++;
      let settled = false;
      try {
        const [sing, server] = await Promise.all([
          HttpUtil.get('/panel/api/setting/singbox/status') as Promise<
            ApiMsg<{ installed: boolean; running: boolean }>
          >,
          HttpUtil.get('/panel/api/server/status') as Promise<
            ApiMsg<{ core?: { running?: 'sing-box' } }>
          >,
        ]);
        if (cancelled) return;
        if (sing.success && sing.obj) {
          const coreRunning =
            (server.success && server.obj?.core?.running === 'sing-box') || sing.obj.running;
          setInstalled(sing.obj.installed);
          setRunning(Boolean(coreRunning));
          settled = !sing.obj.installed || Boolean(coreRunning);
        }
      } catch {
        // Keep the last confirmed status during a temporary connection failure.
      } finally {
        inFlight = false;
        // Stop after one minute; reopening the tab or saving starts a fresh check.
        if (!cancelled && !settled && attempts < 12) timer = setTimeout(() => void refresh(), 5000);
      }
    };

    const onSaved = () => {
      attempts = 0;
      void refresh();
    };
    void refresh();
    window.addEventListener('xui-core-settings-saved', onSaved);
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
      window.removeEventListener('xui-core-settings-saved', onSaved);
    };
  }, [active]);

  return <CoreSelection installed={installed} running={running} />;
}
