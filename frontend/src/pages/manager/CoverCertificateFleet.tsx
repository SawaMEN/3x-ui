import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Card, Select, Space, Table, Tag, Typography } from 'antd';
import type { NodeRecord } from '@/schemas/node';
import { HttpUtil } from '@/utils';

type Site = { nodeId: number; domain: string };
type Fleet = {
  sourceNodeId: number;
  domain: string;
  targets: { nodeId: number; state: string; expiresAt: number; error?: string }[];
};

export default function CoverCertificateFleet({
  nodes,
  defaultRecipientId,
}: {
  nodes: NodeRecord[];
  defaultRecipientId?: number;
}) {
  const { i18n } = useTranslation();
  const ru = i18n.language.startsWith('ru');
  const t = (r: string, e: string) => (ru ? r : e);
  const client = useQueryClient();
  const [selected, setSelected] = useState<string>();
  const [recipients, setRecipients] = useState<number[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [saved, setSaved] = useState(false);
  const name = (id: number) =>
    id === 0
      ? t('Основной сервер', 'Main server')
      : nodes.find((n) => n.id === id)?.name || `#${id}`;
  const sites = useQuery({
    queryKey: ['manager', 'cover-sites'],
    queryFn: async (): Promise<Site[]> => {
      const res = await HttpUtil.get<Site[]>('/panel/api/domains/cover-sites', undefined, {
        silent: true,
      });
      if (!res?.success)
        throw new Error(res?.msg || t('Не удалось загрузить сайты', 'Unable to load sites'));
      return res.obj || [];
    },
    refetchInterval: 30_000,
  });
  const fleet = useQuery({
    queryKey: ['manager', 'certificate-fleet'],
    queryFn: async (): Promise<Fleet[]> => {
      const res = await HttpUtil.get<Fleet[]>('/panel/api/domains/certificates/fleet', undefined, {
        silent: true,
      });
      if (!res?.success)
        throw new Error(
          res?.msg || t('Не удалось загрузить доставку', 'Unable to load delivery status'),
        );
      return res.obj || [];
    },
    refetchInterval: 5_000,
  });
  const choices = new Map<string, Site>();
  for (const site of sites.data || []) choices.set(`${site.nodeId}:${site.domain}`, site);
  for (const row of fleet.data || [])
    choices.set(`${row.sourceNodeId}:${row.domain}`, {
      nodeId: row.sourceNodeId,
      domain: row.domain,
    });
  const site = selected ? choices.get(selected) : undefined;
  const choose = (key: string) => {
    setSelected(key);
    const item = choices.get(key);
    const existing = fleet.data?.find(
      (row) => row.sourceNodeId === item?.nodeId && row.domain === item?.domain,
    );
    setRecipients(
      existing
        ? existing.targets.map((target) => target.nodeId)
        : defaultRecipientId !== undefined && defaultRecipientId !== item?.nodeId
          ? [defaultRecipientId]
          : [],
    );
    setSaved(false);
    setError('');
  };
  const save = async () => {
    if (!site || saving) return;
    setSaving(true);
    setSaved(false);
    setError('');
    try {
      const res = await HttpUtil.post(
        '/panel/api/domains/certificates/fleet',
        { sourceNodeId: site.nodeId, domain: site.domain, nodeIds: recipients },
        { silent: true },
      );
      if (!res?.success) throw new Error(res?.msg || t('Не удалось сохранить', 'Unable to save'));
      await client.invalidateQueries({ queryKey: ['manager', 'certificate-fleet'] });
      void client.invalidateQueries({ queryKey: ['settings', 'managed-sni'] });
      setSaved(true);
    } catch (err) {
      setError(String(err));
    } finally {
      setSaving(false);
    }
  };
  const recipientOptions = [
    { value: 0, label: name(0) },
    ...nodes
      .filter(
        (n) =>
          n.id > 0 &&
          n.enable &&
          !n.transitive &&
          n.scheme === 'https' &&
          n.tlsVerifyMode !== 'skip',
      )
      .map((n) => ({
        value: n.id,
        label: `${name(n.id)}${n.status !== 'online' ? t(' (ожидает подключения)', ' (offline)') : ''}`,
      })),
  ].filter((n) => n.value !== site?.nodeId);
  // Keep saved unavailable nodes visible so recipients can always be removed.
  for (const id of recipients)
    if (!recipientOptions.some((n) => n.value === id))
      recipientOptions.push({ value: id, label: name(id) });
  return (
    <Card
      size="small"
      title={t('Сертификат сайта для других нод', 'Share a site certificate with nodes')}
    >
      <Space orientation="vertical" style={{ width: '100%' }}>
        <Typography.Paragraph>
          {t(
            'Выберите свой сайт-заглушку и ноды. Панель доставит сертификат и ключ по защищённому соединению, а после продления обновит копии автоматически. DNS остаётся направленным на сервер сайта; DNS-01 на основном сервере не нужен.',
            'Choose your cover site and recipient nodes. The panel securely delivers the certificate and key and updates copies after renewal. DNS keeps pointing to the website server; DNS-01 on the main server is unnecessary.',
          )}
        </Typography.Paragraph>
        <Select
          aria-label={t('Сайт-источник сертификата', 'Certificate source site')}
          placeholder={t('Выберите сайт и сервер', 'Choose site and server')}
          style={{ width: '100%' }}
          value={selected}
          onChange={choose}
          loading={sites.isLoading}
          disabled={saving || fleet.isPending || fleet.isError}
          showSearch={{
            filterOption: (input, option) =>
              (option?.label ?? '').toLowerCase().includes(input.toLowerCase()),
          }}
          options={[...choices].map(([value, item]) => ({
            value,
            label: `${item.domain} — ${name(item.nodeId)}`,
          }))}
        />
        <Select
          mode="multiple"
          maxCount={32}
          aria-label={t('Ноды-получатели', 'Recipient nodes')}
          placeholder={t('На какие ноды доставлять сертификат', 'Nodes to receive the certificate')}
          style={{ width: '100%' }}
          value={recipients}
          onChange={(value) => {
            setRecipients(value);
            setSaved(false);
          }}
          options={recipientOptions}
          disabled={!site || saving}
        />
        <Button
          type="primary"
          onClick={() => void save()}
          loading={saving}
          disabled={!site || fleet.isPending || fleet.isError}
        >
          {recipients.length
            ? t('Подключить и обновлять автоматически', 'Connect and update automatically')
            : t('Отключить автоматическую доставку', 'Stop automatic delivery')}
        </Button>
        <Typography.Text type="secondary">
          {t(
            'Выпуск и продление выполняет сервер сайта. После доставки панель применяет конфигурацию ноды; активные подключения могут кратковременно прерваться. В подключении выберите этот SNI — пути сертификата подставятся автоматически.',
            'The website server issues and renews the certificate. Delivery reapplies the node configuration and may briefly interrupt connections. Select this SNI in the inbound to fill certificate paths automatically.',
          )}
        </Typography.Text>
        {saved && (
          <Alert
            type="success"
            showIcon
            title={
              recipients.length
                ? t(
                    'Автоматическая доставка подключена. Дождитесь статуса «Готово» в таблице.',
                    'Automatic delivery connected. Wait for Ready status in the table.',
                  )
                : t(
                    'Автоматическая доставка отключена. Установленные сертификаты сохранены.',
                    'Automatic delivery stopped. Installed certificates are retained.',
                  )
            }
          />
        )}
        {(error || sites.error || fleet.error) && (
          <Alert type="error" showIcon title={error || String(sites.error || fleet.error)} />
        )}
        <Table
          size="small"
          pagination={false}
          scroll={{ x: 560 }}
          dataSource={(fleet.data || []).flatMap((row) =>
            row.targets.map((target) => ({
              ...target,
              domain: row.domain,
              sourceNodeId: row.sourceNodeId,
              key: `${row.sourceNodeId}:${row.domain}:${target.nodeId}`,
            })),
          )}
          columns={[
            { title: t('Сайт', 'Site'), dataIndex: 'domain' },
            { title: t('Источник', 'Source'), dataIndex: 'sourceNodeId', render: name },
            { title: t('Нода', 'Node'), dataIndex: 'nodeId', render: name },
            {
              title: t('Состояние', 'Status'),
              dataIndex: 'state',
              render: (state: string) => (
                <Tag
                  color={state === 'ready' ? 'green' : 'gold'}
                  title={
                    state === 'retrying'
                      ? t(
                          'Проверьте доступность сайта и ноды, действующий сертификат на сервере сайта и защищённое HTTPS-подключение. Основная панель, источник и получатели должны быть обновлены.',
                          'Check source and recipient connectivity, a valid certificate on the website server and verified HTTPS. Update the main panel, source and recipient nodes.',
                        )
                      : undefined
                  }
                >
                  {state === 'ready'
                    ? t('Готово', 'Ready')
                    : state === 'retrying'
                      ? t('Повторяем автоматически', 'Retrying automatically')
                      : t('В очереди', 'Queued')}
                </Tag>
              ),
            },
            {
              title: t('Действует до', 'Expires'),
              dataIndex: 'expiresAt',
              render: (date: number) =>
                date ? new Date(date * 1000).toLocaleDateString(ru ? 'ru-RU' : 'en-US') : '—',
            },
          ]}
        />
      </Space>
    </Card>
  );
}
