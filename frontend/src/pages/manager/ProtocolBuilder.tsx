import { useMemo, useState } from 'react';
import {
  Alert,
  AutoComplete,
  Button,
  Card,
  Checkbox,
  Collapse,
  Input,
  Space,
  Tag,
  Typography,
} from 'antd';
import { RadarChartOutlined } from '@ant-design/icons';
import type { NodeRecord } from '@/schemas/node';
import { runSNIScan } from '@/pages/inbounds/form/security/sni-scanner';
import { nodeSupportsProtocol } from '@/lib/xray/node-protocols';
import { buildManagerRecipes, catalogGroups, managerCatalog, MAX_MANAGER_ENTRIES } from './catalog';
import type { Subscription } from './schema';

interface Props {
  nodes: NodeRecord[];
  domains: string[];
  currentCount: number;
  russian: boolean;
  onApply: (entries: Subscription['entries'], replace: boolean) => void;
}

export default function ProtocolBuilder({ nodes, domains, currentCount, russian, onApply }: Props) {
  const [selectedPresets, setSelectedPresets] = useState<string[]>([]);
  const [nodeIds, setNodeIds] = useState<number[]>([0]);
  const [sniByPreset, setSniByPreset] = useState<Record<string, string>>({});
  const [targetByPreset, setTargetByPreset] = useState<Record<string, string>>({});
  const [verifiedSniByNode, setVerifiedSniByNode] = useState<Record<number, string>>({});
  const [scanning, setScanning] = useState<number | null>(null);
  const [scanError, setScanError] = useState('');
  const [replace, setReplace] = useState(true);
  const [message, setMessage] = useState('');
  const t = (ru: string, en: string) => (russian ? ru : en);

  const onlineNodes = useMemo(
    () => nodes.filter((node) => node.enable && node.status !== 'offline' && !node.transitive),
    [nodes],
  );
  const selectedItems = useMemo(
    () => managerCatalog.filter((item) => selectedPresets.includes(item.id)),
    [selectedPresets],
  );
  const planned = useMemo(
    () =>
      buildManagerRecipes({
        presets: selectedPresets,
        nodeIds,
        nodes,
        domains,
        sniByPreset,
        targetByPreset,
        verifiedSniByNode,
      }),
    [selectedPresets, nodeIds, nodes, domains, sniByPreset, targetByPreset, verifiedSniByNode],
  );
  const total = planned.entries.length + (replace ? 0 : currentCount);
  const lacksRealitySni = selectedItems.some(
    (item) =>
      item.sniMode === 'reality' &&
      (sniByPreset[item.id] || 'auto') === 'auto' &&
      domains.length === 0 &&
      !targetByPreset[item.id] &&
      nodeIds.some((id) => !verifiedSniByNode[id]),
  );
  const unavailable = planned.skipped.length;
  const candidates = [...new Set(domains.map((v) => v.trim()).filter(Boolean))];

  const scanNode = async (nodeId: number) => {
    if (scanning !== null || candidates.length === 0) return;
    setScanning(nodeId);
    setScanError('');
    try {
      // The scan is executed on the selected node. Its result is a suggestion,
      // never a guarantee that another node or network path behaves identically.
      const results = await runSNIScan(
        nodeId || undefined,
        'reality',
        candidates.join('\n'),
        'candidates',
      );
      const ranked = results
        .filter(
          (row) =>
            row.feasible && row.certValid && candidates.includes(row.host) && row.latencyMs >= 0,
        )
        .sort((a, b) => a.latencyMs - b.latencyMs);
      if (ranked.length === 0) {
        setVerifiedSniByNode((current) => {
          const next = { ...current };
          delete next[nodeId];
          return next;
        });
        setScanError(
          t(
            'Для выбранной ноды не найдено подтверждённых SNI.',
            'No verified SNI found on this node.',
          ),
        );
      } else {
        setVerifiedSniByNode((current) => ({ ...current, [nodeId]: ranked[0].host }));
      }
    } catch (error) {
      setScanError(String(error));
    } finally {
      setScanning(null);
    }
  };
  const apply = () => {
    setMessage('');
    if (selectedPresets.length === 0 || nodeIds.length === 0) {
      setMessage(
        t('Выберите протоколы и хотя бы одну ноду.', 'Choose protocols and at least one node.'),
      );
      return;
    }
    if (planned.tooMany || total > MAX_MANAGER_ENTRIES) {
      setMessage(
        t(
          'Превышен лимит в 100 подключений на подписку.',
          'Maximum 100 connections per subscription.',
        ),
      );
      return;
    }
    if (planned.entries.length === 0) {
      setMessage(
        t(
          'Нет совместимых комбинаций протоколов и нод.',
          'There are no compatible node/protocol pairs.',
        ),
      );
      return;
    }
    if (lacksRealitySni) {
      setMessage(
        t(
          'Для REALITY укажите SNI, сайт-заглушку или добавьте SNI в список доменов.',
          'REALITY requires an SNI, a camouflage target, or configured SNI domains.',
        ),
      );
      return;
    }
    onApply(planned.entries, replace);
    setMessage(
      t(
        'Подключения добавлены в черновик. Проверьте их и выполните предварительную проверку.',
        'Recipes added to draft. Review them and run preflight before generation.',
      ),
    );
  };
  return (
    <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
      <Alert
        showIcon
        type="info"
        title={t(
          'Конструктор: отметьте протоколы и ноды, затем сформируйте подключения.',
          'Choose protocols and nodes, then create connection recipes.',
        )}
        description={t(
          'Доступность проверяется для каждой ноды. SNI не бывает универсально «лучшим»: для REALITY используйте проверку сайта на выбранной ноде. HTTPS/TLS требует сертификата именно на этой ноде.',
          'Compatibility is per node. There is no universally best SNI: scan REALITY targets from the chosen node. HTTPS/TLS needs a certificate on that node.',
        )}
      />
      <Typography.Title level={5} style={{ margin: 0 }}>
        {t('1. Протоколы', '1. Protocols')}
      </Typography.Title>
      <Collapse
        defaultActiveKey={['reality', 'tls']}
        items={catalogGroups.map((group) => {
          const items = managerCatalog.filter((item) => item.group === group.id);
          return {
            key: group.id,
            label: `${russian ? group.nameRu : group.name} (${
              items.filter((item) => selectedPresets.includes(item.id)).length
            }/${items.length})`,
            children: (
              <Space orientation="vertical" size="small" style={{ width: '100%' }}>
                {items.map((item) => {
                  const possibleNodes = nodeIds.filter(
                    (id) =>
                      id === 0 ||
                      onlineNodes.some(
                        (node) => node.id === id && nodeSupportsProtocol(node, item.protocol),
                      ),
                  );
                  return (
                    <div
                      key={item.id}
                      style={{
                        borderBottom: '1px solid var(--ant-color-border-secondary)',
                        paddingBottom: 8,
                      }}
                    >
                      <Checkbox
                        checked={selectedPresets.includes(item.id)}
                        onChange={(ev) =>
                          setSelectedPresets((old) =>
                            ev.target.checked
                              ? [...old, item.id]
                              : old.filter((id) => id !== item.id),
                          )
                        }
                      >
                        <strong>{russian ? item.nameRu : item.name}</strong>
                      </Checkbox>
                      <Typography.Paragraph
                        type="secondary"
                        style={{ margin: '3px 0 4px 24px', fontSize: 12 }}
                      >
                        {russian ? item.descriptionRu : item.description}
                      </Typography.Paragraph>
                      {selectedPresets.includes(item.id) && (
                        <div
                          style={{
                            marginLeft: 24,
                            display: 'flex',
                            flexDirection: 'column',
                            gap: 8,
                          }}
                        >
                          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                            {t('Подходящих выбранных нод:', 'Compatible selected nodes:')}{' '}
                            {possibleNodes.length}/{nodeIds.length}
                          </Typography.Text>
                          {item.sniMode !== 'none' && (
                            <div>
                              <Typography.Text style={{ fontSize: 12 }}>
                                {item.sniMode === 'reality'
                                  ? t('SNI для REALITY', 'REALITY SNI')
                                  : t(
                                      'TLS SNI (должен совпадать с сертификатом)',
                                      'TLS SNI (must match certificate)',
                                    )}
                              </Typography.Text>
                              <AutoComplete
                                style={{ width: '100%' }}
                                value={
                                  sniByPreset[item.id] ?? (item.sniMode === 'reality' ? 'auto' : '')
                                }
                                onChange={(value) =>
                                  setSniByPreset((v) => ({ ...v, [item.id]: value }))
                                }
                                options={[
                                  ...(item.sniMode === 'reality'
                                    ? [
                                        {
                                          value: 'auto',
                                          label: t(
                                            'Автоматически (по проверке ноды или из списка)',
                                            'Automatic (node scan or domain list)',
                                          ),
                                        },
                                      ]
                                    : []),
                                  ...candidates.map((domain) => ({ value: domain })),
                                ]}
                                placeholder={t('SNI или автоматический выбор', 'SNI or automatic')}
                              />
                            </div>
                          )}
                          {item.sniMode === 'reality' && (
                            <div>
                              <Typography.Text style={{ fontSize: 12 }}>
                                {t(
                                  'Сайт-заглушка (REALITY Target)',
                                  'Camouflage website (REALITY Target)',
                                )}
                              </Typography.Text>
                              <Input
                                value={targetByPreset[item.id] || ''}
                                placeholder="example.com:443"
                                onChange={(ev) =>
                                  setTargetByPreset((v) => ({ ...v, [item.id]: ev.target.value }))
                                }
                              />
                              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                                {t(
                                  'Пусто — используется выбранный SNI. Это внешний TLS-сайт, а не сайт, который панель размещает на VPS.',
                                  'Leave empty to use SNI as target. This is an external TLS endpoint, not a website hosted by the panel.',
                                )}
                              </Typography.Text>
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  );
                })}
              </Space>
            ),
          };
        })}
      />
      <Typography.Title level={5} style={{ margin: 0 }}>
        {t('2. Ноды и проверка SNI', '2. Nodes and SNI check')}
      </Typography.Title>
      <Card size="small">
        <Space orientation="vertical" style={{ width: '100%' }}>
          {[
            { id: 0, name: t('Локальная панель', 'Local panel') },
            ...onlineNodes.map((node) => ({ id: node.id, name: node.name || `#${node.id}` })),
          ].map((node) => {
            const selected = nodeIds.includes(node.id);
            const known = verifiedSniByNode[node.id];
            return (
              <div
                key={node.id}
                style={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 10 }}
              >
                <Checkbox
                  checked={selected}
                  onChange={(ev) => {
                    setNodeIds((current) =>
                      ev.target.checked
                        ? [...current, node.id]
                        : current.filter((id) => id !== node.id),
                    );
                  }}
                >
                  {node.name}
                </Checkbox>
                {selected && known && (
                  <Tag color="success">
                    {t('Проверен:', 'Verified:')} {known}
                  </Tag>
                )}
                {selected && candidates.length > 0 && (
                  <Button
                    size="small"
                    icon={<RadarChartOutlined />}
                    loading={scanning === node.id}
                    disabled={scanning !== null}
                    onClick={() => void scanNode(node.id)}
                  >
                    {t('Проверить SNI', 'Scan SNI')}
                  </Button>
                )}
              </div>
            );
          })}
        </Space>
      </Card>
      {scanError && (
        <Alert
          type="warning"
          showIcon
          title={scanError}
          closable={{ onClose: () => setScanError('') }}
        />
      )}
      <Typography.Title level={5} style={{ margin: 0 }}>
        {t('3. Создание черновика', '3. Create draft')}
      </Typography.Title>
      <Checkbox checked={replace} onChange={(ev) => setReplace(ev.target.checked)}>
        {t(
          'Заменить текущий список подключений (не удаляет действующие до публикации)',
          'Replace current draft entries (published connections remain until the new generation is applied)',
        )}
      </Checkbox>
      <Space wrap>
        <Tag>
          {t('Комбинаций:', 'Pairs:')} {planned.entries.length}
        </Tag>
        <Tag>
          {t('Пропущено:', 'Skipped:')} {unavailable}
        </Tag>
        <Tag>
          {t('Будет в подписке:', 'Total entries:')} {total}/{MAX_MANAGER_ENTRIES}
        </Tag>
      </Space>
      {planned.skipped.length > 0 && (
        <Alert
          showIcon
          type="warning"
          title={t('Некоторые комбинации недоступны', 'Some pairs are unavailable')}
          description={
            <ul style={{ margin: 0, paddingLeft: 20 }}>
              {planned.skipped.slice(0, 12).map((line, i) => (
                <li key={i}>{line}</li>
              ))}
              {planned.skipped.length > 12 && <li>+{planned.skipped.length - 12}</li>}
            </ul>
          }
        />
      )}
      {message && (
        <Alert
          showIcon
          type={message.includes('черновик') || message.includes('draft') ? 'success' : 'warning'}
          title={message}
        />
      )}
      <Button
        type="primary"
        block
        disabled={scanning !== null || selectedPresets.length === 0}
        onClick={apply}
      >
        {t('Сформировать подключения', 'Generate connection recipes')}
      </Button>
    </Space>
  );
}
