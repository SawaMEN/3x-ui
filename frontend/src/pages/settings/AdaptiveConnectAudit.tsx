import { Alert, Button, Card, Space, Table, Tag, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { useSubBalancersQuery } from '@/api/queries/useSubBalancersQuery';
import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { auditAdaptiveGroups, type AuditIssue } from './adaptive-connect-audit';

const labels: Record<AuditIssue, [string, string]> = {
  disabled: ['Group disabled', 'Группа отключена'],
  'static-strategy': ['No health-aware strategy', 'Стратегия без учёта доступности'],
  'insufficient-members': ['Fewer than two eligible members', 'Меньше двух подходящих подключений'],
  'unavailable-members': ['Missing, disabled or unsupported inbound', 'Отсутствующий, отключённый или неподдерживаемый inbound'],
  'shared-node': ['One hosting node: common failure possible', 'Один узел: возможен общий отказ'],
  'node-diversity-unverified': ['Independent public IPs not verified', 'Независимость публичных IP не проверена'],
  'single-transport': ['Transport diversity not established', 'Разнообразие транспортов не подтверждено'],
};

export default function AdaptiveConnectAudit() {
  const { i18n } = useTranslation();
  const ru = (i18n.resolvedLanguage || i18n.language || '').startsWith('ru');
  const l = (en: string, russian: string) => ru ? russian : en;
  const groups = useSubBalancersQuery();
  const inbounds = useInboundOptions();
  const ready = groups.fetched && !groups.fetchError && inbounds.isSuccess;
  const rows = ready ? auditAdaptiveGroups(groups.balancers, inbounds.data ?? []) : [];

  return (
    <Card size="small" title="Adaptive Connect">
      <Space orientation="vertical" style={{ width: '100%' }}>
        <Alert type="info" showIcon title={l(
          'Target: Hiddify. This audit examines Xray JSON subscription groups. Native Hiddify subscriptions currently emit individual connections, not these groups. Adaptive runtime integration is pending.',
          'Целевая платформа — Hiddify. Аудит проверяет группы подписок Xray JSON. Нативная подписка Hiddify пока передаёт отдельные подключения, а не эти группы. Интеграция адаптивного алгоритма предстоит.',
        )} />
        <Typography.Text type="secondary">{l(
          'Review health-aware strategies and independent failure domains. A configured node does not prove a distinct IP, subnet or provider.',
          'Проверьте стратегии с учётом доступности и независимость резервов. Разные узлы не гарантируют разные IP, подсети или провайдеров.',
        )}</Typography.Text>
        <Button loading={groups.loading || inbounds.isFetching} onClick={() => {
          void groups.refetch();
          void inbounds.refetch();
        }}>{l('Refresh audit','Обновить аудит')}</Button>
        {(groups.fetchError || inbounds.isError) && <Alert type="error" title={l('Could not load configuration','Не удалось загрузить конфигурацию')} />}
        {ready && rows.length === 0 && <Alert type="warning" title={l(
          'No subscription balancers configured. Create a group below.',
          'Нет балансировщиков подписок. Создайте группу ниже.',
        )} />}
        {ready && rows.length > 0 && <Table size="small" rowKey="id" pagination={false} scroll={{ x: true }}
          dataSource={rows} columns={[
            {title:l('Group','Группа'),dataIndex:'name'},
            {title:l('Eligible members','Подключения'),dataIndex:'members'},
            {title:l('Nodes','Узлы'),dataIndex:'nodes'},
            {title:l('Findings','Слабые места'),render:(_, row)=><Space wrap>
              {row.issues.map((issue)=><Tag key={issue} color="orange">{labels[issue][ru ? 1 : 0]}</Tag>)}
              {row.issues.length === 0 && <Tag>{l('No configuration warning','Нет замечаний к конфигурации')}</Tag>}
            </Space>},
          ]} />}
      </Space>
    </Card>
  );
}
