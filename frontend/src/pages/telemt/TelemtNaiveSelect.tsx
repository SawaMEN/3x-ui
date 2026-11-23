import { useEffect, useState } from 'react';
import { Select } from 'antd';
import { useTranslation } from 'react-i18next';

import { HttpUtil } from '@/utils';
import { InboundOptionsSchema, type InboundOption } from '@/schemas/client';

type Props = {
  id?: string;
  value?: number;
  onChange?: (value?: number) => void;
  disabled?: boolean;
};

export default function TelemtNaiveSelect(props: Props) {
  const { i18n } = useTranslation();
  const ru = i18n.resolvedLanguage?.startsWith('ru') ?? false;
  const [inbounds, setInbounds] = useState<InboundOption[]>([]);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    const abort = new AbortController();
    void HttpUtil.get('/panel/api/inbounds/options', undefined, {
      signal: abort.signal,
      silent: true,
    })
      .then((response) => {
        if (abort.signal.aborted) return;
        const parsed = InboundOptionsSchema.safeParse(response?.obj);
        if (response?.success && parsed.success) {
          setInbounds(parsed.data);
        } else {
          setFailed(true);
        }
      })
      .catch(() => {
        if (!abort.signal.aborted) setFailed(true);
      })
      .finally(() => {
        if (!abort.signal.aborted) setLoading(false);
      });
    return () => abort.abort();
  }, []);

  const options = inbounds
    .filter(
      (inbound) =>
        inbound.enable &&
        !inbound.nodeId &&
        inbound.protocol === 'naive' &&
        inbound.network === 'tcp' &&
        inbound.port !== 443 &&
        /^(?:127\.|::1$)/.test(inbound.listen ?? ''),
    )
    .map((inbound) => ({
      value: inbound.id,
      label: `${inbound.remark || inbound.tag || '#' + inbound.id} · ${inbound.port}`,
    }));

  return (
    <Select<number>
      {...props}
      allowClear
      loading={loading}
      options={options}
      placeholder={ru ? 'Выберите NaiveProxy' : 'Select NaiveProxy'}
      notFoundContent={
        failed
          ? ru
            ? 'Не удалось загрузить подключения'
            : 'Could not load inbounds'
          : ru
            ? 'Нет подходящих локальных TCP-подключений'
            : 'No eligible local TCP inbounds'
      }
    />
  );
}
