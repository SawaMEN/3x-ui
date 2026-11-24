import { useEffect, useState } from 'react';
import { Alert, Button, Select, Space, Typography } from 'antd';
import { useTranslation } from 'react-i18next';

import { HttpUtil } from '@/utils';
import { InboundOptionsSchema, type InboundOption } from '@/schemas/client';

type Props = {
  id?: string;
  value?: number[];
  onChange?: (value: number[]) => void;
  nodeId?: number | null;
  webHosts?: string[];
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

  const local = inbounds.filter((inbound) => (inbound.nodeId ?? null) === (props.nodeId ?? null));
  const webHosts = new Set((props.webHosts ?? []).map((host) => host.trim().toLowerCase()));
  const eligible = local.filter((inbound) => {
    const sni = inbound.sharedIngressSni?.toLowerCase();
    return Boolean(inbound.enable && sni && !inbound.sharedIngressError && !webHosts.has(sni));
  });
  const counts = new Map<string, number>();
  for (const inbound of eligible) {
    const sni = inbound.sharedIngressSni!.toLowerCase();
    counts.set(sni, (counts.get(sni) ?? 0) + 1);
  }
  const selected = props.value ?? [];
  const selectedSet = new Set(selected);
  const eligibleIds = new Set(eligible.map((inbound) => inbound.id));
  const localIds = new Set(local.map((inbound) => inbound.id));
  const chosenBySNI = new Map<string, number[]>();
  for (const inbound of eligible) {
    if (!selectedSet.has(inbound.id)) continue;
    const sni = inbound.sharedIngressSni!.toLowerCase();
    chosenBySNI.set(sni, [...(chosenBySNI.get(sni) ?? []), inbound.id]);
  }
  // Keep an explicit, unambiguous manual choice when adding other compatible
  // routes. Never guess which of two inbounds with the same SNI should win.
  const automatic = eligible.filter((inbound) => {
    const sni = inbound.sharedIngressSni!.toLowerCase();
    return (
      counts.get(sni) === 1 || (chosenBySNI.get(sni)?.length === 1 && selectedSet.has(inbound.id))
    );
  });
  const autoIds = [
    ...automatic.filter((inbound) => selectedSet.has(inbound.id)),
    ...automatic.filter((inbound) => !selectedSet.has(inbound.id)),
  ]
    .slice(0, 64)
    .map((inbound) => inbound.id);
  const supportedProtocols = new Set([
    'vless', 'vmess', 'trojan', 'shadowsocks', 'naive', 'naiveproxy',
    'vetroff', 'vetroff-proxy', 'anytls', 'trusttunnel', 'masque', 'shadowtls',
  ]);
  const options = local
    .filter((inbound) =>
      inbound.sharedIngressSni || selectedSet.has(inbound.id) ||
      supportedProtocols.has(inbound.protocol?.toLowerCase() ?? ''),
    )
    .map((inbound) => {
      const sni = inbound.sharedIngressSni?.toLowerCase();
      const valid = Boolean(sni && eligibleIds.has(inbound.id));
      return {
        value: inbound.id,
        label: `${inbound.remark || inbound.tag || '#' + inbound.id} · ${inbound.protocol} · ${inbound.port}${sni ? ' · ' + sni : ''}${valid && inbound.port === 443 ? (ru ? ' · требуется перенос' : ' · migration required') : ''}${!valid ? ' · ' + (inbound.sharedIngressError || (ru ? 'SNI недоступен или конфликтует' : 'SNI unavailable or conflicting')) : ''}`,
        title:
          inbound.sharedIngressError ||
          (sni && webHosts.has(sni)
            ? ru
              ? 'SNI совпадает с WEB-доменом'
              : 'SNI conflicts with a WEB hostname'
            : sni),
        disabled:
          !selectedSet.has(inbound.id) && (!valid || (sni !== undefined && chosenBySNI.has(sni))),
      };
    });
  for (const id of selected) {
    if (!localIds.has(id)) {
      options.push({
        value: id,
        label: `#${id}`,
        title: ru ? 'Подключение не найдено' : 'Inbound not found',
        disabled: false,
      });
    }
  }
  const blocked = selected.filter((id) => !eligibleIds.has(id));
  const selectedConflict = [...chosenBySNI.values()].some((ids) => ids.length > 1);
  const ambiguous = eligible.filter(
    (inbound) => (counts.get(inbound.sharedIngressSni!.toLowerCase()) ?? 0) > 1,
  ).length;
  const udp = local.filter((inbound) => inbound.udp443 && inbound.enable);

  return (
    <Space orientation="vertical" size="small" style={{ width: '100%' }}>
      <Select<number[]>
        id={props.id}
        value={props.value ?? []}
        onChange={props.onChange}
        disabled={props.disabled}
        mode="multiple"
        maxCount={64}
        allowClear
        loading={loading}
        style={{ width: '100%' }}
        showSearch={{ optionFilterProp: 'label' }}
        options={options}
        placeholder={ru ? 'Выберите TCP-подключения' : 'Select TCP inbounds'}
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
      <Space wrap>
        <Button
          size="small"
          disabled={props.disabled || loading || failed || automatic.length === 0}
          onClick={() => props.onChange?.(autoIds)}
        >
          {ru ? 'Выбрать совместимые автоматически' : 'Select compatible automatically'}
        </Button>
        <Typography.Text type="secondary">
          {ru
            ? `Выбрано ${selected.length}/64 · без конфликтов ${automatic.length}`
            : `Selected ${selected.length}/64 · conflict-free ${automatic.length}`}
        </Typography.Text>
      </Space>
      {eligible.some((inbound) => selectedSet.has(inbound.id) && inbound.port === 443) && (
        <Alert
          type="info"
          showIcon
          title={ru
            ? 'Подключения на TCP 443 требуют подтверждённого переноса. Проверьте план настройки ниже.'
            : 'Inbounds on TCP 443 require a confirmed migration. Review the setup preview below.'}
        />
      )}
      {ambiguous > 0 && (
        <Alert
          type="warning"
          showIcon
          title={
            ru
              ? `У ${ambiguous} подключений повторяется SNI. Выберите не более одного на каждое имя вручную.`
              : `${ambiguous} inbounds share an SNI. Select at most one per name manually.`
          }
        />
      )}
      {!loading && !failed && (blocked.length > 0 || selectedConflict) && (
        <Alert
          type="error"
          showIcon
          title={
            ru
              ? 'Выбранные подключения требуют исправления или удаления из списка.'
              : 'Selected inbounds must be corrected or removed.'
          }
          description={blocked
            .map(
              (id) =>
                local.find((inbound) => inbound.id === id)?.sharedIngressError || `#${id}: SNI`,
            )
            .join(' · ')}
        />
      )}
      {udp.length > 0 && (
        <Typography.Text type="secondary">
          {ru ? 'UDP 443 (независимо от TCP): ' : 'UDP 443 (independent of TCP): '}
          {udp.map((inbound) => inbound.remark || inbound.tag || `#${inbound.id}`).join(', ')}
        </Typography.Text>
      )}
    </Space>
  );
}
