import { useEffect, useMemo, useState } from 'react';
import { Card, Table, Tag, Typography } from 'antd';
import { HttpUtil } from '@/utils';
import type { RouteRecord, RoutingEngine } from '../routing/quick-routing';
import { managedNodeRoutes, type ManagedNodeRoute } from './managed-node-routes';

interface NodeRouteTarget {
  tag: string;
  nodeName: string;
  remark: string;
  protocol: string;
  available: boolean;
  reason?: string;
}

interface Props {
  engine: RoutingEngine;
  rules: RouteRecord[];
}

// Managed cascade destinations exist only in the generated core configuration.
// Show their public metadata here, not the service credentials or the raw
// generated outbound (both are intentionally absent from the editor template).
export default function ManagedNodeRouteOutbounds({ engine, rules }: Props) {
  const routes = useMemo(() => managedNodeRoutes(rules, engine), [rules, engine]);
  const [targets, setTargets] = useState<NodeRouteTarget[] | null>(null);
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading');

  useEffect(() => {
    if (routes.length === 0) return;
    let active = true;
    setStatus('loading');
    void HttpUtil.get<NodeRouteTarget[]>(
      '/panel/api/xray/node-route/targets?core=' + encodeURIComponent(engine),
      undefined,
      { silent: true },
    )
      .then((response) => {
        if (!active) return;
        if (response?.success && Array.isArray(response.obj)) {
          setTargets(response.obj);
          setStatus('ready');
        } else {
          setStatus('error');
        }
      })
      .catch(() => {
        if (active) setStatus('error');
      });
    return () => {
      active = false;
    };
  }, [engine, routes.length]);

  if (routes.length === 0) return null;

  const byTag = new Map((targets || []).map((target) => [target.tag, target]));
  return (
    <Card title="Каскадные исходящие подключения" size="small">
      <Typography.Paragraph type="secondary">
        Эти подключения автоматически создаются в рабочей конфигурации ядра по активным
        правилам маршрутизации. Вручную редактировать их не требуется. После изменения
        маршрутов сохраните настройки. Доступность ноды не означает, что трафик уже
        проходит через каскад.
      </Typography.Paragraph>
      <Table<ManagedNodeRoute>
        size="small"
        rowKey="tag"
        dataSource={routes}
        pagination={false}
        scroll={{ x: 680 }}
        columns={[
          {
            title: 'Tag исходящего',
            dataIndex: 'tag',
            render: (tag: string) => <Tag color="blue">{tag}</Tag>,
          },
          {
            title: 'Удалённая нода',
            render: (_: unknown, row: ManagedNodeRoute) => {
              const target = byTag.get(row.tag);
              if (!target) return '—';
              return [target.nodeName, target.remark || target.protocol]
                .filter(Boolean)
                .join(' · ');
            },
          },
          {
            title: 'Протокол',
            render: (_: unknown, row: ManagedNodeRoute) =>
              byTag.get(row.tag)?.protocol || '—',
          },
          { title: 'Правил', dataIndex: 'ruleCount', width: 90 },
          {
            title: 'Нода',
            render: (_: unknown, row: ManagedNodeRoute) => {
              if (status === 'loading') return <Tag>Загрузка</Tag>;
              if (status === 'error') return <Tag>Нет данных</Tag>;
              const target = byTag.get(row.tag);
              if (!target) return <Tag color="warning">Не найдена</Tag>;
              return target.available ? (
                <Tag color="success">Доступна для маршрута</Tag>
              ) : (
                <Tag color="error" title={target.reason || undefined}>
                  {target.reason || 'Недоступна'}
                </Tag>
              );
            },
          },
        ]}
      />
    </Card>
  );
}
