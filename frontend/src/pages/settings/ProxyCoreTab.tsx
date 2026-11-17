import { useCallback, useEffect, useState } from 'react';
import { Alert } from 'antd';
import { useQueryClient } from '@tanstack/react-query';
import { keys } from '@/api/queryKeys';
import { HttpUtil } from '@/utils';
import CoreSelection, { type CoreKind, type CoreStatus } from './CoreSelection';

interface ApiMsg<T> {
  success?: boolean;
  msg?: string;
  obj?: T;
}
export default function ProxyCoreTab({ active }: { active: boolean }) {
  const queryClient = useQueryClient();
  const [cores, setCores] = useState<CoreStatus[]>([]);
  const [busy, setBusy] = useState<CoreKind | null>(null);
  const [error, setError] = useState('');
  const refresh = useCallback(async () => {
    const result = (await HttpUtil.get('/panel/api/setting/cores')) as ApiMsg<{
      cores: CoreStatus[];
    }>;
    if (!result?.success || !result.obj)
      throw new Error(result?.msg || 'Unable to load core status');
    setCores(result.obj.cores);
  }, []);
  useEffect(() => {
    if (!active) return;
    let cancelled = false;
    const load = () => {
      void refresh().catch((err) => {
        if (!cancelled) setError(String(err));
      });
    };
    load();
    window.addEventListener('xui-core-settings-saved', load);
    return () => {
      cancelled = true;
      window.removeEventListener('xui-core-settings-saved', load);
    };
  }, [active, refresh]);
  const action = async (kind: CoreKind, operation: 'install' | 'select' | 'delete') => {
    if (busy) return;
    setBusy(kind);
    setError('');
    try {
      const result = (await HttpUtil.post(
        `/panel/api/setting/cores/${kind}/${operation}`,
        {},
        { timeout: 330000 },
      )) as ApiMsg<{ selected?: CoreKind }>;
      if (!result?.success) throw new Error(result?.msg || 'Core operation failed');
      if (operation === 'select') {
        void queryClient.invalidateQueries({ queryKey: keys.settings.all() });
        void queryClient.invalidateQueries({ queryKey: ['server'] });
        window.dispatchEvent(new CustomEvent('xui-core-selected', { detail: kind }));
      }
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  };
  return (
    <>
      {error && <Alert type="error" title={error} showIcon style={{ marginBottom: 12 }} />}
      <CoreSelection
        cores={cores}
        busy={busy}
        onAction={(kind, operation) => void action(kind, operation)}
      />
    </>
  );
}
