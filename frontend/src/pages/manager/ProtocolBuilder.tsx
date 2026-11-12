import MainNodeDNS01Setup from '@/components/MainNodeDNS01Setup';
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
import type { RealityScanResult } from '@/generated/types';
import {
  bestManagerAutoSNI,
  isAutoUsableManagerSNI,
  rankManagerSNISuggestions,
} from './sni-suggestions';
import { runSNIScan } from '@/pages/inbounds/form/security/sni-scanner';
import { nodeSupportsProtocol } from '@/lib/xray/node-protocols';
import { useManagedSniDomains } from '@/api/queries/useManagedSniDomains';
import {
  managerCertificateName,
  managerTLSCertificateForSni,
  managerTLSCertificatesForNode,
} from './certificate-recipes';
import { HttpUtil } from '@/utils';
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
  const [backupSniByNode, setBackupSniByNode] = useState<Record<number, string[]>>({});
  const [scanResultsByNode, setScanResultsByNode] = useState<Record<number, RealityScanResult[]>>(
    {},
  );
  const [scanning, setScanning] = useState<number | null>(null);
  const [scanError, setScanError] = useState('');
  const [replace, setReplace] = useState(true);
  const [allowUnconfiguredTLS, setAllowUnconfiguredTLS] = useState(false);
  const [tlsSniByNode, setTlsSniByNode] = useState<Record<number, string>>({});
  const [certificateFilesByPair, setCertificateFilesByPair] = useState<
    Record<string, { certFile: string; keyFile: string }>
  >({});
  const [acmeEmail, setAcmeEmail] = useState('');
  const [acmeConsent, setAcmeConsent] = useState(false);
  const [issuingCertificates, setIssuingCertificates] = useState(false);
  const [issueMessage, setIssueMessage] = useState('');
  const {
    data: certificates = [],
    isFetching: certsFetching,
    isError: certsError,
    refetch: refetchCertificates,
  } = useManagedSniDomains();
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
        backupSniByNode,
        managedCertificates: certificates,
        certificateFilesByPair,
        tlsSniByNode,
        allowUnconfiguredTLS,
      }),
    [
      selectedPresets,
      nodeIds,
      nodes,
      domains,
      sniByPreset,
      targetByPreset,
      verifiedSniByNode,
      backupSniByNode,
      certificates,
      certificateFilesByPair,
      tlsSniByNode,
      allowUnconfiguredTLS,
    ],
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
  const candidates = [
    ...new Set(
      [...domains, ...certificates.map((site) => site.name)]
        .filter((name): name is string => typeof name === 'string')
        .map((name) => name.trim())
        .filter(Boolean),
    ),
  ];

  const scanNode = async (nodeId: number) => {
    if (scanning !== null) return;
    setScanning(nodeId);
    setScanError('');
    setVerifiedSniByNode((current) => {
      const next = { ...current };
      delete next[nodeId];
      return next;
    });
    setScanResultsByNode((current) => ({ ...current, [nodeId]: [] }));
    setBackupSniByNode((current) => ({ ...current, [nodeId]: [] }));
    try {
      // Reuse the panel scanner (neighbor discovery, candidate discovery,
      // TLS 1.3/H2/X25519 and repeated stability probes).
      // Neighboring IPs must not be replaced by unrelated DNS destinations.
      const results = await runSNIScan(nodeId || undefined, 'reality', '', 'combined');
      const ranked = rankManagerSNISuggestions(results);
      setScanResultsByNode((current) => ({ ...current, [nodeId]: ranked.slice(0, 10) }));
      const best = bestManagerAutoSNI(ranked);
      if (best) {
        setVerifiedSniByNode((current) => ({ ...current, [nodeId]: best }));
      } else {
        setScanError(
          t(
            'Нет стабильных SNI с подтверждённым совпадением DNS и IP. Результаты поиска по соседним IP ниже; для автоматического применения необходима отдельная проверка.',
            'No stable SNI with matching DNS and tested IP. Neighboring IP discoveries are shown below; automatic deployment requires separate verification.',
          ),
        );
      }
    } catch (error) {
      setScanError(String(error));
    } finally {
      setScanning(null);
    }
  };
  const issueNodeCertificates = async () => {
    if (issuingCertificates) return;
    if (!acmeConsent || !acmeEmail.trim()) {
      setIssueMessage(
        t(
          'Укажите контактный email и подтвердите владение доменом и условия Let’s Encrypt.',
          'Enter an ACME email and confirm domain ownership and Let’s Encrypt terms.',
        ),
      );
      return;
    }
    const targets = new Map<string, { nodeId: number; domain: string }>();
    const skipped: string[] = [];
    for (const nodeId of nodeIds) {
      for (const item of selectedItems) {
        if (item.sniMode !== 'tls') continue;
        const selected = tlsSniByNode[nodeId] ?? sniByPreset[item.id] ?? '';
        const domain = managerCertificateName(selected);
        if (!domain || domain === 'auto' || domain === 'inherit') {
          skipped.push(`${item.name} · #${nodeId}`);
          continue;
        }
        if (managerTLSCertificateForSni(certificates, nodeId, domain)) continue;
        targets.set(`${nodeId}|${domain}`, { nodeId, domain });
      }
    }
    if (targets.size === 0) {
      setIssueMessage(
        skipped.length
          ? t(
              'Укажите конкретные TLS-домены для протоколов или каждой ноды.',
              'Select explicit TLS hostnames for protocols or nodes.',
            )
          : t(
              'Все выбранные TLS-сертификаты уже найдены.',
              'All selected TLS certificates are already available.',
            ),
      );
      return;
    }
    if (targets.size > 30) {
      setIssueMessage(
        t(
          'За один запуск поддерживается не более 30 сертификатов. Разделите ноды на группы.',
          'Maximum 30 certificates per request. Split nodes into groups.',
        ),
      );
      return;
    }
    setIssuingCertificates(true);
    const successes: string[] = [];
    const errors: string[] = [];
    try {
      for (const target of targets.values()) {
        try {
          const result = await HttpUtil.post(
            target.nodeId === 0
              ? '/panel/api/manager/domains/local-certificate'
              : '/panel/api/manager/domains/cover-site',
            {
              nodeId: target.nodeId,
              domain: target.domain,
              email: acmeEmail.trim(),
              acceptTerms: true,
            },
            { headers: { 'Content-Type': 'application/json' }, silent: true },
          );
          if (result?.success) {
            const payload = result.obj as { state?: string } | undefined;
            successes.push(`${target.domain} (#${target.nodeId}): ${payload?.state ?? 'pending'}`);
          } else {
            errors.push(`${target.domain} (#${target.nodeId}): ${result?.msg ?? 'failed'}`);
          }
        } catch (error) {
          errors.push(`${target.domain} (#${target.nodeId}): ${String(error)}`);
        }
      }
      await refetchCertificates();
      setIssueMessage(
        [
          ...successes,
          ...errors,
          ...(skipped.length
            ? [
                t(
                  'Для некоторых протоколов не указан TLS-домен.',
                  'Some protocols have no explicit TLS hostname.',
                ),
              ]
            : []),
          t(
            'Новые сертификаты могут выпускаться асинхронно. Обновите список, затем сформируйте подключения.',
            'New certificates may be issued asynchronously. Refresh the list before generating recipes.',
          ),
        ].join(' · '),
      );
    } finally {
      setIssuingCertificates(false);
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
                                value={sniByPreset[item.id] ?? 'auto'}
                                onChange={(value) =>
                                  setSniByPreset((v) => ({ ...v, [item.id]: value }))
                                }
                                options={[
                                  {
                                    value: 'auto',
                                    label:
                                      item.sniMode === 'tls'
                                        ? t(
                                            'Автоматически (сертификат для каждой ноды)',
                                            'Automatic (certificate on each node)',
                                          )
                                        : t(
                                            'Автоматически (по проверке ноды или из списка)',
                                            'Automatic (node scan or domain list)',
                                          ),
                                  },
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
        {t('2. Ноды, сертификаты и проверка SNI', '2. Nodes, certificates and SNI check')}
      </Typography.Title>
      {certsError && (
        <Alert
          showIcon
          type="warning"
          title={t(
            'Не удалось получить список управляемых сертификатов. TLS-подключения без подтверждённых сертификатов не будут добавлены.',
            'Managed certificates are unavailable. TLS recipes without verified certificates will be skipped.',
          )}
        />
      )}
      <Typography.Text type="secondary">
        {t(
          'Для TLS автоматически выбирается действующий сертификат с самой ноды. Сертификаты и пути разных нод не смешиваются.',
          'TLS recipes use a valid certificate from their own node. Paths are never shared between nodes.',
        )}
        {certsFetching ? ' …' : ''}
      </Typography.Text>
      <Card size="small">
        <Space orientation="vertical" style={{ width: '100%' }}>
          {[
            { id: 0, name: t('Локальная панель', 'Local panel') },
            ...onlineNodes.map((node) => ({ id: node.id, name: node.name || `#${node.id}` })),
          ].map((node) => {
            const selected = nodeIds.includes(node.id);
            const known = verifiedSniByNode[node.id];
            const ownedCertificates = managerTLSCertificatesForNode(certificates, node.id);
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
                {selected && (backupSniByNode[node.id]?.length ?? 0) > 0 && (
                  <Tag color="processing">
                    {t('Резервных SNI:', 'Backup SNIs:')} {backupSniByNode[node.id].length}
                  </Tag>
                )}
                {selected && (
                  <Tag>
                    {t('TLS-сертификаты:', 'TLS certificates:')} {ownedCertificates.length}
                  </Tag>
                )}
                {selected && selectedItems.some((item) => item.sniMode === 'tls') && (
                  <AutoComplete
                    style={{ minWidth: 240, maxWidth: '100%' }}
                    value={tlsSniByNode[node.id] ?? 'inherit'}
                    onChange={(value) =>
                      setTlsSniByNode((current) => {
                        const next = { ...current };
                        if (value === 'inherit') delete next[node.id];
                        else next[node.id] = value;
                        return next;
                      })
                    }
                    options={[
                      {
                        value: 'inherit',
                        label: t('TLS: настройки протокола', 'TLS: protocol selection'),
                      },
                      {
                        value: 'auto',
                        label: t('TLS: автоматически на ноде', 'TLS: automatic on node'),
                      },
                      ...[
                        ...new Set([
                          ...ownedCertificates.map((site) => site.name),
                          ...certificates
                            .filter((site) => site.source === 'remote')
                            .map((site) => site.name),
                        ]),
                      ].map((name) => ({
                        value: name,
                        label: name,
                      })),
                    ]}
                  />
                )}
                {selected && ownedCertificates.length > 0 && (
                  <div style={{ width: '100%' }}>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      {t(
                        'Действующие сертификаты ноды (срок в днях; без срока — старая версия API):',
                        'Node certificates (days remaining; no date means an older node API):',
                      )}
                    </Typography.Text>
                    <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap', marginTop: 4 }}>
                      {ownedCertificates.slice(0, 6).map((site) => {
                        const days = site.expiresAt
                          ? Math.max(0, Math.ceil((site.expiresAt * 1000 - Date.now()) / 86400000))
                          : null;
                        return (
                          <Tag
                            key={site.name}
                            color={days !== null && days <= 14 ? 'warning' : undefined}
                          >
                            {site.name}
                            {days !== null ? ` · ${days} ${t('дн.', 'days')}` : ''}
                            {site.issuer ? ` · ${site.issuer}` : ''}
                          </Tag>
                        );
                      })}
                      {ownedCertificates.length > 6 && <Tag>+{ownedCertificates.length - 6}</Tag>}
                    </div>
                  </div>
                )}
                {selected && selectedItems.some((item) => item.sniMode === 'tls') && (
                  <Collapse
                    size="small"
                    style={{ width: '100%' }}
                    items={selectedItems
                      .filter((item) => item.sniMode === 'tls')
                      .map((item) => {
                        const key = `${item.id}|${node.id}`;
                        const manual = certificateFilesByPair[key] || { certFile: '', keyFile: '' };
                        const requestedSni =
                          tlsSniByNode[node.id] ?? sniByPreset[item.id] ?? 'auto';
                        const matched = managerTLSCertificateForSni(
                          certificates,
                          node.id,
                          requestedSni,
                        );
                        const setFile = (field: 'certFile' | 'keyFile', value: string) =>
                          setCertificateFilesByPair((prev) => ({
                            ...prev,
                            [key]: { ...prev[key], [field]: value },
                          }));
                        return {
                          key,
                          label: item.name,
                          children: (
                            <Space orientation="vertical" style={{ width: '100%' }}>
                              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                                {t(
                                  'Пути проверяются на выбранной ноде при предварительной проверке. Не указывайте пути с другого сервера.',
                                  'File paths are validated during preflight on the selected node. Do not use paths from another host.',
                                )}
                              </Typography.Text>
                              <Typography.Text type="secondary">certFile</Typography.Text>
                              <Input
                                aria-label={`${item.name} certificate path #${node.id}`}
                                value={manual.certFile}
                                onChange={(event) => setFile('certFile', event.target.value)}
                                placeholder={
                                  matched?.certFile ||
                                  t('Путь к fullchain.pem', 'Path to fullchain.pem')
                                }
                              />
                              <Typography.Text type="secondary">keyFile</Typography.Text>
                              <Input
                                aria-label={`${item.name} private key path #${node.id}`}
                                value={manual.keyFile}
                                onChange={(event) => setFile('keyFile', event.target.value)}
                                placeholder={
                                  matched?.keyFile || t('Путь к privkey.pem', 'Path to privkey.pem')
                                }
                              />
                            </Space>
                          ),
                        };
                      })}
                  />
                )}
                {selected && (
                  <Button
                    size="small"
                    icon={<RadarChartOutlined />}
                    loading={scanning === node.id}
                    disabled={scanning !== null}
                    onClick={() => void scanNode(node.id)}
                  >
                    {t('Интеллектуальный поиск SNI', 'Intelligent SNI search')}
                  </Button>
                )}
                {selected && (scanResultsByNode[node.id]?.length ?? 0) > 0 && (
                  <div style={{ width: '100%', display: 'flex', flexDirection: 'column', gap: 6 }}>
                    <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                      {t(
                        'Найденные SNI с этой ноды. Для автоматического применения требуется совпадение DNS и проверенного IP.',
                        'SNI discovered from this node. Automatic selection requires DNS to match the verified IP.',
                      )}
                    </Typography.Text>
                    {scanResultsByNode[node.id].slice(0, 5).map((row) => (
                      <div
                        key={`${row.host}|${row.target}`}
                        style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 6 }}
                      >
                        <Tag color={isAutoUsableManagerSNI(row) ? 'success' : 'default'}>
                          {isAutoUsableManagerSNI(row)
                            ? t('DNS совпадает', 'DNS matches')
                            : t('Проверен только IP', 'IP only')}
                        </Tag>
                        <Typography.Text style={{ fontSize: 12 }}>
                          {row.host} · {row.ip} · {row.latencyMs} ms
                          {row.attempts ? ` · ${row.successes ?? 0}/${row.attempts}` : ''}
                        </Typography.Text>
                        {isAutoUsableManagerSNI(row) && (
                          <Button
                            size="small"
                            onClick={() =>
                              setVerifiedSniByNode((current) => ({
                                ...current,
                                [node.id]: row.host,
                              }))
                            }
                          >
                            {t('Выбрать', 'Select')}
                          </Button>
                        )}
                        {isAutoUsableManagerSNI(row) && row.host !== known && (
                          <Button
                            size="small"
                            type={
                              backupSniByNode[node.id]?.includes(row.host) ? 'primary' : 'default'
                            }
                            onClick={() =>
                              setBackupSniByNode((current) => {
                                const list = current[node.id] || [];
                                const next = list.includes(row.host)
                                  ? list.filter((host) => host !== row.host)
                                  : [...list, row.host].slice(0, 6);
                                return { ...current, [node.id]: next };
                              })
                            }
                          >
                            {backupSniByNode[node.id]?.includes(row.host)
                              ? t('В резерве', 'In reserve')
                              : t('Добавить в резерв', 'Add backup')}
                          </Button>
                        )}
                      </div>
                    ))}
                  </div>
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
        {t('3. Автоматический выпуск сертификатов', '3. Automatic certificate issuance')}
      </Typography.Title>
      <Card size="small">
        <Space orientation="vertical" style={{ width: '100%' }}>
          <Typography.Text type="secondary">
            {t(
              'Для TLS выберите домен. Основная нода выпускает собственный сертификат через DNS-01 без занятия портов 80/443; удалённые ноды используют Domain Front. Приватные ключи между серверами не копируются.',
              'Choose a TLS hostname per protocol or node. The main node issues its own certificate via DNS-01 without public ports 80/443; remote nodes use Domain Front. Private keys are never copied.',
            )}
          </Typography.Text>
          {nodeIds.includes(0) && <MainNodeDNS01Setup />}
          <Input
            value={acmeEmail}
            onChange={(event) => setAcmeEmail(event.target.value)}
            type="email"
            maxLength={254}
            placeholder={t('Email для ACME', 'ACME contact email')}
          />
          <Checkbox
            checked={acmeConsent}
            onChange={(event) => setAcmeConsent(event.target.checked)}
          >
            {t(
              'Я контролирую указанные домены и принимаю условия Let’s Encrypt.',
              'I control the selected hostnames and accept the Let’s Encrypt terms.',
            )}
          </Checkbox>
          <Button
            loading={issuingCertificates}
            disabled={issuingCertificates || selectedItems.every((item) => item.sniMode !== 'tls')}
            onClick={() => void issueNodeCertificates()}
          >
            {t(
              'Выпустить недостающие TLS-сертификаты на нодах',
              'Issue missing TLS certificates on selected nodes',
            )}
          </Button>
          {issueMessage && <Alert showIcon type="info" title={issueMessage} />}
        </Space>
      </Card>
      <Typography.Title level={5} style={{ margin: 0 }}>
        {t('4. Создание черновика', '4. Create draft')}
      </Typography.Title>
      <Checkbox
        checked={allowUnconfiguredTLS}
        onChange={(ev) => setAllowUnconfiguredTLS(ev.target.checked)}
      >
        {t(
          'Разрешить TLS без найденного сертификата (только черновик — необходимо настроить вручную)',
          'Allow TLS without managed certificates (draft only — manual configuration required)',
        )}
      </Checkbox>
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
