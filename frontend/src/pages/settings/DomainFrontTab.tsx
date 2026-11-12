import { useMemo, useState } from 'react';
import { Link } from 'react-router';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { keys } from '@/api/queryKeys';
import { useDomainFrontStatus } from '@/api/queries/useDomainFrontStatus';
import { useServerDraft } from '@/hooks/useServerDraft';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  AutoComplete,
  Button,
  Card,
  Dropdown,
  Checkbox,
  Collapse,
  Form,
  Input,
  InputNumber,
  List,
  Modal,
  Select,
  Segmented,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import { ReloadOutlined, QuestionCircleOutlined, MoreOutlined } from '@ant-design/icons';
import { HttpUtil } from '@/utils';
import { antdRule } from '@/utils/antdRule';
import { DomainFrontStatusSchema, DomainFrontCheckSchema } from '@/generated/zod';
import type { DomainFrontCheck } from '@/generated/zod';
import {
  DomainConfigFormSchema,
  DomainFormSchema,
  defaultDomain,
  defaultDomainConfig,
} from '@/schemas/domain-front';
import type { DomainConfigForm, DomainForm } from '@/schemas/domain-front';
import DomainOverview from './DomainOverview';
import CertificateInventoryPanel from './CertificateInventoryPanel';
import { DomainPreflightReportSchema } from '@/schemas/domain-preflight';
import type { DomainPreflightReport } from '@/schemas/domain-preflight';
import type { DomainPreset } from './DomainOverview';
import { getDomainHealth } from './domain-health';
import type { AllSetting } from '@/models/setting';
import { SettingListItem } from '@/components/ui';
import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { useManagedSniDomains } from '@/api/queries/useManagedSniDomains';
import { redirectSiteOptions } from './redirect-sites';
import { onNumber } from '@/utils/onNumber';
import { rootRedirectError } from './root-redirect';
import { TelemtWebCatalogs } from '@/schemas/telemt-web-catalog';
import CoverSiteHTMLEditor, { coverSiteHTMLValidation, starterCoverSiteHTML } from '@/components/CoverSiteHTMLEditor';

type DomainSection = 'domains' | 'certificates' | 'sni' | 'settings';

type RedirectSiteCheck = {
  url: string;
  available: boolean;
  status: number;
  pathStatus: number;
  latencyMs: number;
  sniLatencyMs?: number;
  error?: string;
};

type RedirectSiteScan = {
  recommended: string;
  checkedAt: number;
  sites: RedirectSiteCheck[];
};

export default function DomainFrontTab({
  allSetting,
  updateSetting,
  saveSettings,
  settingsDirty = false,
}: {
  saveSettings?: () => Promise<boolean | undefined>;
  settingsDirty?: boolean;
  allSetting?: AllSetting;
  updateSetting?: (patch: Partial<AllSetting>) => void;
}) {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const statusQuery = useDomainFrontStatus();
  const { data: managedSniDomains = [] } = useManagedSniDomains();
  const telemtSites = managedSniDomains.filter((site) => site.source === 'telemt');
  const status = statusQuery.data;
  const config = status?.config;
  const server = useMemo(
    () => (config ? DomainConfigFormSchema.parse(config) : undefined),
    [config],
  );
  const {
    draft: localDraft,
    setDraft,
    isDirty: dirty,
    markSaved,
  } = useServerDraft(
    server,
    (value) => structuredClone(value),
    (left, right) => JSON.stringify(left) === JSON.stringify(right),
  );
  const draft = localDraft ?? defaultDomainConfig;
  const loading = statusQuery.isPending;
  const initialized = server !== undefined;
  const { data: options = [] } = useInboundOptions();
  const [checks, setChecks] = useState<Record<string, DomainFrontCheck>>({});
  const [checking, setChecking] = useState<string>();
  const [auditing, setAuditing] = useState(false);
  const [auditDone, setAuditDone] = useState(0);
  const [auditTotal, setAuditTotal] = useState(0);
  const [domainSearch, setDomainSearch] = useState('');
  const [domainFilter, setDomainFilter] = useState('all');
  const [section, setSection] = useState<DomainSection>('domains');
  const [expanded, setExpanded] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [preflighting, setPreflighting] = useState(false);
  const [preflightReport, setPreflightReport] = useState<DomainPreflightReport>();
  const [preflightDraft, setPreflightDraft] = useState('');
  const [scanningRedirect, setScanningRedirect] = useState(false);
  const [redirectScan, setRedirectScan] = useState<RedirectSiteScan>();
  const [separatePublicPort, setSeparatePublicPort] = useState(false);
  const [editing, setEditing] = useState<number>();
  const [primaryDomain, setPrimaryDomain] = useState(false);
  const [advancedEditorOpen, setAdvancedEditorOpen] = useState(false);
  const [dnsToken, setDnsToken] = useState('');
  const [savingDNSToken, setSavingDNSToken] = useState(false);
  const dnsCredential = useQuery({
    queryKey: ['settings', 'domains', 'dns-credential', draft.dns01Provider || 'cloudflare'],
    enabled: section === 'settings',
    retry: false,
    queryFn: async () => {
      const response = await HttpUtil.get(
        `/panel/api/domains/dns/credential?provider=${draft.dns01Provider || 'cloudflare'}`,
        undefined,
        { silent: true },
      );
      if (!response?.success) throw new Error(response?.msg || 'Cannot load DNS token status');
      return response.obj as { configured: boolean; source?: string };
    },
  });
  const [form] = Form.useForm<DomainForm>();
  const [messageApi, holder] = message.useMessage();
  const mode = Form.useWatch('mode', form);
  const siteCatalog = Form.useWatch('siteCatalog', form);
  const tls = Form.useWatch('tls', form);
  const label = (key: string, values?: Record<string, string | number>) =>
    t(`pages.settings.domains.${key}`, values);
  const dnsHelp = (key: string, ru: string, en: string) => (
    <Space size={5}>
      {label('dns01.' + key)}
      <Tooltip title={i18n.language.startsWith('ru') ? ru : en}>
        <QuestionCircleOutlined aria-label={key + ' DNS-01 help'} style={{ cursor: 'help' }} />
      </Tooltip>
    </Space>
  );
  const renewalLabel = (value: string) =>
    value === 'external' || value === 'manual' ? label(`renewal.${value}`) : value;
  const certificateError = (value: string) =>
    value === 'acme_schedule_missing' ? label('acmeScheduleMissing') : value;

  function load() {
    return statusQuery.refetch();
  }

  async function updateDNSToken(clear = false) {
    if (savingDNSToken) return;
    setSavingDNSToken(true);
    try {
      const response = await HttpUtil.post(
        '/panel/api/domains/dns/credential',
        clear
          ? { provider: draft.dns01Provider || 'cloudflare', clear: true }
          : { provider: draft.dns01Provider || 'cloudflare', token: dnsToken },
        { silent: true },
      );
      if (!response?.success) throw new Error(response?.msg || label('dns01.saveError'));
      setDnsToken('');
      messageApi.success(label(clear ? 'dns01.cleared' : 'dns01.saved'));
      void dnsCredential.refetch();
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : label('dns01.saveError'));
    } finally {
      setSavingDNSToken(false);
    }
  }

  function update(patch: Partial<DomainConfigForm>) {
    setDraft((prev) => ({ ...(prev ?? defaultDomainConfig), ...patch }));
  }
  const redirectDomains = [
    ...new Set(
      [
        ...draft.domains
          .filter(
            (domain) =>
              domain.enabled &&
              domain.mode !== 'sni_only' &&
              domain.mode !== 'shared_sni' &&
              domain.mode !== 'cert_only',
          )
          .map((domain) => domain.name),
        allSetting?.webDomain || '',
      ].filter(Boolean),
    ),
  ];
  const redirectError = allSetting?.rootRedirectEnable
    ? rootRedirectError(allSetting.rootRedirectURL, redirectDomains)
    : '';
  async function preflight(config: DomainConfigForm) {
    const response = await HttpUtil.post('/panel/api/domains/preflight', config, {
      silent: true,
      timeout: 15_000,
    });
    if (!response?.success) throw new Error(response?.msg || t('somethingWentWrong'));
    const report = DomainPreflightReportSchema.parse(response.obj);
    setPreflightReport(report);
    setPreflightDraft(JSON.stringify(config));
    return report;
  }

  async function checkReadiness() {
    const parsed = DomainConfigFormSchema.safeParse(draft);
    if (!parsed.success) {
      messageApi.error(t(parsed.error.issues[0].message));
      return;
    }
    setPreflighting(true);
    try {
      const report = await preflight(parsed.data);
      if (!report.ready) messageApi.warning(label('preflight.blocked'));
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('somethingWentWrong'));
    } finally {
      setPreflighting(false);
    }
  }

  async function apply() {
    if (redirectError) {
      messageApi.error(redirectError);
      return;
    }
    const parsed = DomainConfigFormSchema.safeParse(draft);
    if (!parsed.success) {
      messageApi.error(t(parsed.error.issues[0].message));
      return;
    }
    if (dirty) {
      setPreflighting(true);
      try {
        const report = await preflight(parsed.data);
        if (!report.ready) {
          messageApi.error(label('preflight.blocked'));
          return;
        }
        const warnings = report.checks.filter((check) => check.severity === 'warning');
        if (warnings.length > 0) {
          Modal.confirm({
            title: label('preflight.confirm'),
            content: label('preflight.confirmDetails', { count: warnings.length }),
            okText: label('preflight.continue'),
            cancelText: t('cancel'),
            onOk: () => commitParsed(parsed.data),
          });
          return;
        }
      } catch (error) {
        messageApi.error(error instanceof Error ? error.message : t('somethingWentWrong'));
        return;
      } finally {
        setPreflighting(false);
      }
    }
    await commitParsed(parsed.data);
  }

  async function commitParsed(parsed: DomainConfigForm) {
    const domainFirst = dirty && settingsDirty && !!allSetting?.rootRedirectEnable;
    let committed = false;
    setBusy(true);
    try {
      if (!domainFirst && settingsDirty && saveSettings && !(await saveSettings())) return;
      if (!dirty) {
        await load();
        messageApi.success(
          'Перенаправление применено. Панель доступна только по порту из её настроек.',
        );
        return;
      }
      const msg = await HttpUtil.post('/panel/api/domains/apply', parsed, { silent: true });
      if (!msg?.success) throw new Error(msg?.msg || t('somethingWentWrong'));
      const next = DomainFrontStatusSchema.parse(msg.obj);
      committed = true;
      if (domainFirst && saveSettings && !(await saveSettings()))
        throw new Error('Не удалось сохранить перенаправление.');
      const saved = DomainConfigFormSchema.parse(next.config);
      setDraft(saved);
      markSaved(saved);
      queryClient.setQueryData(keys.settings.domains(), next);
      void queryClient.invalidateQueries({ queryKey: ['settings', 'managed-sni'] });
      void queryClient.invalidateQueries({ queryKey: keys.settings.defaults() });
      void queryClient.invalidateQueries({ queryKey: keys.settings.all() });
      void load();
      messageApi.success(t('pages.settings.domains.applied'));
      if (saved.enabled && saved.subscriptionDomain) void check(saved.subscriptionDomain);
    } catch (e) {
      let detail = e instanceof Error ? e.message : t('somethingWentWrong');
      if (domainFirst && committed && server) {
        const rollback = await HttpUtil.post('/panel/api/domains/apply', server, { silent: true });
        detail += rollback.success
          ? ' Прежние настройки доменов восстановлены.'
          : ` Не удалось восстановить домены: ${rollback.msg}`;
      }
      messageApi.error(detail);
      void load();
    } finally {
      setBusy(false);
    }
  }
  async function findRedirectSite() {
    if (!updateSetting) return;
    setScanningRedirect(true);
    try {
      const response = await HttpUtil.post(
        '/panel/api/domains/recommend',
        {},
        { silent: true, timeout: 75_000 },
      );
      if (!response?.success) throw new Error(response?.msg || t('somethingWentWrong'));
      const scan = response.obj as RedirectSiteScan;
      if (!scan || !Array.isArray(scan.sites) || typeof scan.recommended !== 'string')
        throw new Error(t('pages.settings.redirectScanError'));
      // Discovery is read-only. The operator chooses a destination and
      // explicitly enables/saves redirection separately, if desired.
      setRedirectScan(scan);
      if (!scan.recommended) {
        messageApi.warning(t('pages.settings.redirectScanEmpty'));
      }
    } catch (e) {
      messageApi.error(e instanceof Error ? e.message : t('somethingWentWrong'));
    } finally {
      setScanningRedirect(false);
    }
  }

  async function check(name: string) {
    setChecking(name);
    try {
      const msg = await HttpUtil.post('/panel/api/domains/check', { name }, { silent: true });
      if (!msg?.success) throw new Error(msg?.msg || t('somethingWentWrong'));
      const result = DomainFrontCheckSchema.parse(msg.obj);
      setChecks((prev) => ({ ...prev, [name]: result }));
      setExpanded((prev) => [...new Set([...prev, name])]);
    } catch (e) {
      messageApi.error(e instanceof Error ? e.message : t('somethingWentWrong'));
    } finally {
      setChecking(undefined);
    }
  }
  async function auditDomains() {
    if (!status || dirty || busy || auditing) return;
    // Only saved domains can be checked. Limit concurrency to avoid
    // overwhelming the panel and the DNS/TLS endpoints on larger installations.
    const names = status.config.domains
      .filter((domain) => domain.enabled)
      .map((domain) => domain.name);
    if (names.length === 0) return;
    setAuditing(true);
    setAuditTotal(names.length);
    setAuditDone(0);
    let next = 0;
    let completed = 0;
    let failures = 0;
    try {
      await Promise.all(
        Array.from({ length: Math.min(names.length, 3) }, async () => {
          while (next < names.length) {
            const name = names[next++];
            try {
              const msg = await HttpUtil.post(
                '/panel/api/domains/check',
                { name },
                { silent: true },
              );
              if (!msg?.success) throw new Error(msg?.msg || t('somethingWentWrong'));
              const result = DomainFrontCheckSchema.parse(msg.obj);
              if (result.error) failures++;
              setChecks((prev) => ({ ...prev, [name]: result }));
            } catch (error) {
              failures++;
              setChecks((prev) => ({
                ...prev,
                [name]: {
                  name,
                  addresses: [],
                  checkedAt: Math.floor(Date.now() / 1000),
                  expiresAt: 0,
                  daysLeft: 0,
                  issuer: '',
                  error: error instanceof Error ? error.message : t('somethingWentWrong'),
                },
              }));
            } finally {
              completed++;
              setAuditDone(completed);
            }
          }
        }),
      );
      if (failures > 0)
        messageApi.warning(
          t('pages.settings.domains.workspace.auditWarnings', { count: failures }),
        );
      else messageApi.success(t('pages.settings.domains.workspace.auditSuccess'));
    } finally {
      setAuditing(false);
    }
  }

  function openPreset(preset: DomainPreset) {
    openEditor(-1, preset === 'sni', preset === 'shared');
    if (preset === 'direct') form.setFieldsValue({ mode: 'direct', tls: 'auto' });
    if (preset === 'cdn') form.setFieldsValue({ mode: 'cdn', tls: 'auto' });
  }

  function openEditor(index: number, sniOnly = false, sharedOnly = false) {
    form.resetFields();
    form.setFieldsValue(
      index < 0
        ? {
            ...defaultDomain,
            mode: sharedOnly ? 'shared_sni' : sniOnly ? 'sni_only' : 'sub_link_only',
            tls: sniOnly ? 'certmagic' : 'auto',
            siteCatalog: sniOnly ? 'builtin' : '',
            siteTemplate: sniOnly ? 'notes' : '',
          }
        : {
            ...draft.domains[index],
            ...(draft.domains[index].mode === 'sni_only'
              ? {
                  siteCatalog: draft.domains[index].siteCatalog || 'builtin',
                  siteTemplate: draft.domains[index].siteTemplate || 'notes',
                }
              : {}),
          },
    );
    setPrimaryDomain(
      index < 0
        ? !sniOnly && !sharedOnly && !draft.subscriptionDomain
        : draft.subscriptionDomain === draft.domains[index].name,
    );
    setAdvancedEditorOpen(
      index >= 0 && (draft.domains[index]?.tls !== 'auto' || !draft.domains[index]?.enabled),
    );
    setEditing(index);
  }
  async function saveDomain() {
    const value = await form.validateFields();
    if (value.mode === 'sni_only' && telemtSites.some((site) => site.name === value.name)) {
      messageApi.info(label('telemtAlreadyExists'));
      setEditing(undefined);
      return;
    }
    const old = editing != null && editing >= 0 ? draft.domains[editing] : undefined;
    const domain = DomainFormSchema.parse({
      ...defaultDomain,
      ...value,
      panel: false,
      inboundIds:
        value.mode === 'sub_link_only' ||
        value.mode === 'sni_only' ||
        value.mode === 'shared_sni' ||
        value.mode === 'cert_only'
          ? []
          : (value.inboundIds ?? []),
      origin: value.mode === 'relay' ? value.origin : '',
      siteCatalog: value.mode === 'sni_only' ? value.siteCatalog || '' : '',
      siteTemplate: value.mode === 'sni_only' ? value.siteTemplate || '' : '',
      siteHtml: value.mode === 'sni_only' && value.siteCatalog === 'custom' ? value.siteHtml || '' : '',
      certFile:
        value.tls === 'manual' && value.mode !== 'shared_sni' && value.mode !== 'cert_only'
          ? value.certFile
          : '',
      keyFile:
        value.tls === 'manual' && value.mode !== 'shared_sni' && value.mode !== 'cert_only'
          ? value.keyFile
          : '',
    });
    const domains = [...draft.domains];
    if (editing != null && editing >= 0) domains[editing] = domain;
    else domains.push(domain);
    const canonical =
      primaryDomain &&
      domain.enabled &&
      domain.mode !== 'sni_only' &&
      domain.mode !== 'shared_sni' &&
      domain.mode !== 'cert_only'
        ? domain.name
        : draft.subscriptionDomain === old?.name
          ? ''
          : draft.subscriptionDomain;
    update({
      domains,
      subscriptionDomain: canonical,
      ...(editing === -1 &&
      domain.mode !== 'shared_sni' &&
      !draft.domains.some((item) => item.enabled && item.mode !== 'shared_sni')
        ? { enabled: true }
        : {}),
    });
    setEditing(undefined);
  }
  const date = (epoch: number) =>
    new Intl.DateTimeFormat(i18n.language, { dateStyle: 'medium' }).format(epoch * 1000);
  const filteredDomains = draft.domains.filter((domain) => {
    if (!domain.name.toLowerCase().includes(domainSearch.trim().toLowerCase())) return false;
    if (domainFilter === 'all') return true;
    if (domainFilter === 'attention') {
      const saved =
        status?.config.domains.some(
          (item) => item.name === domain.name && JSON.stringify(item) === JSON.stringify(domain),
        ) ?? false;
      return ['error', 'expiring'].includes(
        getDomainHealth(domain, status, checks[domain.name], saved),
      );
    }
    return domain.mode === domainFilter;
  });
  const compatible = options.filter(
    (o) =>
      o.enable &&
      !o.nodeId &&
      ['ws', 'httpupgrade', 'xhttp', 'splithttp', 'grpc'].includes(o.network || '') &&
      (!o.security || o.security === 'none' || o.security === 'tls'),
  );

  return (
    <Space className="domain-manager" orientation="vertical" size={16} style={{ width: '100%' }}>
      {holder}
      <Space wrap>
        <Button
          type="primary"
          loading={busy || preflighting}
          aria-label={label('apply')}
          disabled={
            (!dirty && !settingsDirty) || loading || !!redirectError || preflighting || busy
          }
          onClick={() => void apply()}
        >
          {label('apply')}
        </Button>
        <Button
          onClick={() => void checkReadiness()}
          disabled={!initialized || preflighting || busy}
          loading={preflighting}
          aria-label={label('preflight.action')}
        >
          {label('preflight.action')}
        </Button>
        <Button icon={<ReloadOutlined />} onClick={() => void load()}>
          {label('refresh')}
        </Button>
        {dirty && <Typography.Text type="secondary">{label('unsaved')}</Typography.Text>}
      </Space>
      {preflightReport && preflightDraft === JSON.stringify(draft) && (
        <Card size="small" title={label('preflight.title')}>
          <Space orientation="vertical" style={{ width: '100%' }}>
            <Alert
              showIcon
              type={
                !preflightReport.ready
                  ? 'error'
                  : preflightReport.checks.some((check) => check.severity === 'warning')
                    ? 'warning'
                    : 'success'
              }
              title={label(preflightReport.ready ? 'preflight.ready' : 'preflight.blocked')}
            />
            <List
              size="small"
              dataSource={preflightReport.checks}
              renderItem={(check) => (
                <List.Item>
                  <Space orientation="vertical" size={2} style={{ width: '100%' }}>
                    <Space wrap>
                      <Tag
                        color={
                          check.severity === 'error'
                            ? 'red'
                            : check.severity === 'warning'
                              ? 'orange'
                              : 'default'
                        }
                      >
                        {label('preflight.severity.' + check.severity)}
                      </Tag>
                      {check.domain && <Typography.Text strong>{check.domain}</Typography.Text>}
                      <Typography.Text>{label('preflight.code.' + check.code)}</Typography.Text>
                    </Space>
                    <Typography.Text type="secondary">{check.message}</Typography.Text>
                    {check.addresses?.length ? (
                      <Typography.Text code>{check.addresses.join(', ')}</Typography.Text>
                    ) : null}
                  </Space>
                </List.Item>
              )}
            />
          </Space>
        </Card>
      )}
      <DomainOverview
        draft={draft}
        status={status}
        checks={checks}
        managedSniCount={managedSniDomains.length}
        dirty={dirty}
        loading={loading}
        auditing={auditing}
        auditDone={auditDone}
        auditTotal={auditTotal}
        onAudit={() => void auditDomains()}
        onAdd={openPreset}
      />
      {statusQuery.error && (
        <Alert
          type="error"
          showIcon
          title={label('gatewayError')}
          description={statusQuery.error.message}
        />
      )}

      {section === 'domains' &&
        draft.enabled &&
        draft.domains.some(
          (item) => item.enabled && item.mode !== 'shared_sni' && item.tls === 'auto',
        ) &&
        (!draft.email || !draft.acceptTerms) && (
          <Card size="small" title={label('workspace.autoTLSSetup')}>
            <Typography.Paragraph type="secondary">
              {label('workspace.autoTLSHint')}
            </Typography.Paragraph>
            <Space wrap align="start" style={{ width: '100%' }}>
              <Input
                type="email"
                aria-label={label('email')}
                placeholder={label('email')}
                value={draft.email}
                onChange={(event) => update({ email: event.target.value })}
                style={{ width: 280, maxWidth: '100%' }}
              />
              <Checkbox
                checked={draft.acceptTerms}
                onChange={(event) => update({ acceptTerms: event.target.checked })}
              >
                {label('terms')}{' '}
                <a
                  href="https://letsencrypt.org/repository/"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  Let's Encrypt
                </a>
              </Checkbox>
            </Space>
          </Card>
        )}
      <div
        className="domain-manager-nav"
        role="navigation"
        aria-label={label('workspace.navigation')}
      >
        <Segmented
          block
          value={section}
          onChange={(value) => setSection(value as DomainSection)}
          options={(['domains', 'certificates', 'sni', 'settings'] as const).map((value) => ({
            value,
            label: label('workspace.section.' + value),
          }))}
        />
      </div>
      {section === 'domains' && (
        <div className="domain-manager-panel" data-testid="domain-panel-domains">
          <Card size="small" className="domain-manager-hosts" title={label('list')}>
            <Space wrap style={{ marginBottom: 12, width: '100%' }}>
              <Input.Search
                allowClear
                placeholder={label('workspace.searchPlaceholder')}
                aria-label={label('workspace.searchPlaceholder')}
                value={domainSearch}
                onChange={(event) => setDomainSearch(event.target.value)}
                style={{ width: 250, maxWidth: '100%' }}
              />
              <Select
                aria-label={label('workspace.filterLabel')}
                value={domainFilter}
                onChange={setDomainFilter}
                style={{ minWidth: 180 }}
                options={[
                  { value: 'all', label: label('workspace.filter.all') },
                  { value: 'attention', label: label('workspace.filter.attention') },
                  ...(
                    [
                      'direct',
                      'cdn',
                      'relay',
                      'sub_link_only',
                      'sni_only',
                      'shared_sni',
                      'cert_only',
                    ] as const
                  ).map((mode) => ({
                    value: mode,
                    label:
                      mode === 'cert_only'
                        ? i18n.language.startsWith('ru')
                          ? 'Только сертификат (DNS-01)'
                          : 'Certificate only (DNS-01)'
                        : label('mode.' + mode),
                  })),
                ]}
              />
              <Typography.Text type="secondary">
                {label('workspace.showing', { count: filteredDomains.length })}
              </Typography.Text>
            </Space>
            <Table
              className="domain-config-table"
              size="small"
              rowKey="name"
              dataSource={filteredDomains}
              pagination={{ pageSize: 10, hideOnSinglePage: true }}
              scroll={{ x: 600 }}
              locale={{ emptyText: label('empty') }}
              columns={[
                {
                  title: label('name'),
                  dataIndex: 'name',
                  render: (name: string, d: DomainForm) => (
                    <Space orientation="vertical" size={2}>
                      <Typography.Text strong>{name}</Typography.Text>
                      {!d.enabled && <Tag>{label('disabled')}</Tag>}
                    </Space>
                  ),
                },
                {
                  title: label('modeLabel'),
                  dataIndex: 'mode',
                  render: (value: string) => (
                    <Typography.Text type="secondary">
                      {value === 'cert_only'
                        ? i18n.language.startsWith('ru')
                          ? 'Только сертификат (DNS-01)'
                          : 'Certificate only (DNS-01)'
                        : label('mode.' + value)}
                    </Typography.Text>
                  ),
                },
                {
                  title: label('workspace.healthLabel'),
                  key: 'health',
                  render: (_value: unknown, d: DomainForm) => {
                    const saved =
                      status?.config.domains.some(
                        (item) =>
                          item.name === d.name && JSON.stringify(item) === JSON.stringify(d),
                      ) ?? false;
                    const health = getDomainHealth(d, status, checks[d.name], saved);
                    const cert = status?.certificates.find((item) => item.name === d.name);
                    const color =
                      health === 'healthy'
                        ? 'success'
                        : health === 'error'
                          ? 'error'
                          : health === 'expiring'
                            ? 'warning'
                            : 'default';
                    return (
                      <Space orientation="vertical" size={2}>
                        <Tag color={color}>{label('workspace.health.' + health)}</Tag>
                        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                          {cert?.expiresAt
                            ? `${label('remaining')}: ${cert.daysLeft} ${label('days')}`
                            : d.mode === 'shared_sni'
                              ? label('remoteSNIInfo')
                              : label('tls.' + d.tls)}
                        </Typography.Text>
                      </Space>
                    );
                  },
                },
                {
                  title: label('actions'),
                  render: (_value: unknown, d: DomainForm) => (
                    <Space size={4}>
                      <Button
                        type="link"
                        size="small"
                        onClick={() =>
                          openEditor(draft.domains.findIndex((item) => item.name === d.name))
                        }
                      >
                        {label('edit')}
                      </Button>
                      {d.mode === 'sni_only' && (
                        <Button
                          type="link"
                          size="small"
                          onClick={() =>
                            openEditor(draft.domains.findIndex((item) => item.name === d.name))
                          }
                        >
                          {i18n.language.startsWith('ru')
                            ? 'Сменить сайт-заглушку'
                            : 'Change cover site'}
                        </Button>
                      )}
                      <Dropdown
                        trigger={['click']}
                        menu={{
                          items: [
                            {
                              key: 'check',
                              label: label('check'),
                              disabled: dirty || !initialized || auditing,
                            },
                            { type: 'divider' },
                            { key: 'remove', label: label('remove'), danger: true },
                          ],
                          onClick: ({ key }) => {
                            if (key === 'check') void check(d.name);
                            if (key === 'remove') {
                              Modal.confirm({
                                title: label('removeConfirm'),
                                okText: label('remove'),
                                okButtonProps: { danger: true },
                                onOk: () =>
                                  update({
                                    domains: draft.domains.filter((item) => item.name !== d.name),
                                    subscriptionDomain:
                                      draft.subscriptionDomain === d.name
                                        ? ''
                                        : draft.subscriptionDomain,
                                  }),
                              });
                            }
                          },
                        }}
                      >
                        <Button
                          icon={<MoreOutlined />}
                          size="small"
                          aria-label={label('workspace.moreActions')}
                          loading={checking === d.name}
                        />
                      </Dropdown>
                    </Space>
                  ),
                },
              ]}
              expandable={{
                expandedRowKeys: expanded,
                onExpandedRowsChange: (keys) => setExpanded(keys.map(String)),
                expandedRowRender: (d) => {
                  const result = checks[d.name];
                  return result ? (
                    <Space orientation="vertical">
                      <Typography.Text>
                        {label('dns')}: {result.addresses.join(', ') || '—'}
                      </Typography.Text>
                      {result.expiresAt > 0 && (
                        <Typography.Text>
                          {label('publicTLS')}: {result.issuer} · {result.daysLeft} {label('days')}{' '}
                          · {date(result.expiresAt)}
                        </Typography.Text>
                      )}
                      {result.error && (
                        <Typography.Text type="danger">{result.error}</Typography.Text>
                      )}
                      <Typography.Text type="secondary">{label('publicTLSDesc')}</Typography.Text>
                    </Space>
                  ) : (
                    <Typography.Text type="secondary">{label('checkHint')}</Typography.Text>
                  );
                },
              }}
            />
          </Card>
        </div>
      )}
      {section === 'certificates' && (
        <div className="domain-manager-panel" data-testid="domain-panel-certificates">
          <Alert
            type="info"
            showIcon
            title={label('automation')}
            description={label('automationDesc')}
          />
          <CertificateInventoryPanel />
          {!!status?.existingCertificates?.length && (
            <Collapse
              items={[
                {
                  key: 'certificates',
                  label: label('existingCertificates'),
                  children: (
                    <>
                      <Typography.Paragraph type="secondary">
                        {label('existingDesc')}
                      </Typography.Paragraph>
                      <Space orientation="vertical">
                        {status.existingCertificates.map((cert) => (
                          <div key={cert.name}>
                            <Typography.Text strong>
                              {label(`source.${cert.source}`)}
                            </Typography.Text>
                            {' · '}
                            {cert.issuer}
                            {' · '}
                            {label('remaining')}: {cert.daysLeft} {label('days')}
                            {' · '}
                            {date(cert.expiresAt)}
                            {' · '}
                            {renewalLabel(cert.renewal)}
                            {cert.autoRenew && <Tag color="green">{label('autoRenew')}</Tag>}
                            <div>
                              <Typography.Text copyable>{cert.certFile}</Typography.Text>
                            </div>
                            {cert.error && (
                              <Typography.Text type="warning">
                                {certificateError(cert.error)}
                              </Typography.Text>
                            )}
                          </div>
                        ))}
                      </Space>
                    </>
                  ),
                },
              ]}
            />
          )}
        </div>
      )}

      {section === 'sni' && (
        <div className="domain-manager-panel" data-testid="domain-panel-sni">
          {telemtSites.length > 0 && (
            <Alert
              type="info"
              showIcon
              title={label('telemtReuseTitle')}
              description={label('telemtReuseDesc')}
            />
          )}
          <Card size="small" title={label('sniCertificates')}>
            <Typography.Paragraph type="secondary">{label('sniCertHint')}</Typography.Paragraph>
            <Space orientation="vertical" size={10} style={{ width: '100%' }}>
              {managedSniDomains.length === 0 &&
                !draft.domains.some(
                  (item) => item.mode === 'sni_only' || item.mode === 'shared_sni',
                ) && <Typography.Text type="secondary">{label('sniEmpty')}</Typography.Text>}
              {managedSniDomains.map((site) => (
                <Space wrap key={`${site.nodeId ?? 'local'}-${site.name}-${site.source}`}>
                  <Typography.Text strong>{site.name}</Typography.Text>
                  <Tag
                    color={
                      site.source === 'telemt' || site.source === 'gateway' ? 'success' : undefined
                    }
                  >
                    {site.source === 'remote' ? label('remoteSNITag') : site.source}
                  </Tag>
                  {site.nodeName && (
                    <Typography.Text type="secondary">{site.nodeName}</Typography.Text>
                  )}
                </Space>
              ))}
              {draft.domains
                .filter(
                  (item) =>
                    (item.mode === 'sni_only' || item.mode === 'shared_sni') &&
                    !managedSniDomains.some((site) => site.name === item.name),
                )
                .map((item) => (
                  <Space wrap key={item.name}>
                    <Typography.Text strong>{item.name}</Typography.Text>
                    <Tag>
                      {item.mode === 'shared_sni' ? label('remoteSNITag') : label('state.unsaved')}
                    </Tag>
                  </Space>
                ))}
            </Space>
          </Card>
        </div>
      )}
      {section === 'settings' && (
        <div className="domain-manager-panel" data-testid="domain-panel-settings">
          <Card size="small" title={label('dns01.title')} style={{ marginBottom: 12 }}>
            <Typography.Paragraph type="secondary">{label('dns01.hint')}</Typography.Paragraph>
            <Form layout="vertical">
              <Form.Item
                label={dnsHelp(
                  'provider',
                  'Выберите того, кто обслуживает DNS-записи домена: Cloudflare или DigitalOcean. Нода может находиться у другого хостинг-провайдера.',
                  'Choose the provider hosting the domain DNS: Cloudflare or DigitalOcean. The node can be hosted elsewhere.',
                )}
              >
                <Select
                  aria-label={label('dns01.provider')}
                  value={draft.dns01Provider || ''}
                  onChange={(value) => {
                    setDnsToken('');
                    update({
                      dns01Provider: value,
                      ...(value !== 'cloudflare' ? { dns01ZoneId: '' } : {}),
                      ...(value === '' ? { dns01Zone: '', dns01Propagation: 0 } : {}),
                    });
                  }}
                  options={[
                    { value: '', label: label('dns01.disabled') },
                    { value: 'cloudflare', label: 'Cloudflare' },
                    { value: 'digitalocean', label: 'DigitalOcean' },
                  ]}
                />
              </Form.Item>
              {(draft.dns01Provider === 'cloudflare' || draft.dns01Provider === 'digitalocean') && (
                <>
                  <Space wrap style={{ width: '100%' }}>
                    <Form.Item
                      label={dnsHelp(
                        'zone',
                        'Основная DNS-зона домена, например example.org для vpn.example.org. Она должна охватывать имена выдаваемых сертификатов.',
                        'Authoritative DNS zone, e.g. example.org for vpn.example.org. It must contain the certificate hostnames.',
                      )}
                    >
                      <Input
                        aria-label={label('dns01.zone')}
                        value={draft.dns01Zone ?? ''}
                        placeholder="example.com"
                        onChange={(event) => update({ dns01Zone: event.target.value })}
                      />
                    </Form.Item>
                    {draft.dns01Provider === 'cloudflare' && (
                      <Form.Item
                        label={dnsHelp(
                          'zoneId',
                          'В Cloudflare откройте домен → Overview → Zone ID (32 символа). Это не идентификатор аккаунта.',
                          'In Cloudflare open the domain → Overview → Zone ID (32 characters). This is not the account ID.',
                        )}
                      >
                        <Input
                          aria-label={label('dns01.zoneId')}
                          value={draft.dns01ZoneId ?? ''}
                          placeholder="32-character Cloudflare ID"
                          onChange={(event) => update({ dns01ZoneId: event.target.value })}
                        />
                      </Form.Item>
                    )}
                    <Form.Item
                      label={dnsHelp(
                        'propagation',
                        'Пауза 0–180 секунд перед проверкой TXT-записи. Увеличьте при медленном распространении DNS; обычно оставьте 0.',
                        'Wait 0–180 seconds before checking the TXT record. Increase only for slow DNS propagation; usually leave 0.',
                      )}
                    >
                      <InputNumber
                        aria-label={label('dns01.propagation')}
                        min={0}
                        max={180}
                        value={draft.dns01Propagation ?? 0}
                        onChange={onNumber((value) => update({ dns01Propagation: value }))}
                      />
                    </Form.Item>
                  </Space>
                  <Typography.Paragraph type="secondary">
                    {dnsCredential.data?.configured
                      ? label('dns01.credentialSaved')
                      : label('dns01.credentialMissing')}
                    {dnsCredential.data?.source === 'environment' &&
                      ' · ' + label('dns01.environment')}
                  </Typography.Paragraph>
                  <Space wrap>
                    {dnsHelp(
                      'token',
                      'Токен API с минимальным доступом к управлению DNS-записями этой зоны. Сохраняется только на основном сервере, не передаётся удалённым нодам.',
                      'API token scoped to edit DNS records in this zone. Stored only on the main server, not distributed to remote nodes.',
                    )}
                    <Input.Password
                      aria-label={label('dns01.token')}
                      placeholder={label('dns01.token')}
                      autoComplete="new-password"
                      value={dnsToken}
                      onChange={(event) => setDnsToken(event.target.value)}
                      style={{ width: 300, maxWidth: '100%' }}
                    />
                    <Button
                      loading={savingDNSToken}
                      disabled={!dnsToken}
                      onClick={() => void updateDNSToken()}
                    >
                      {label('dns01.saveToken')}
                    </Button>
                    <Button
                      danger
                      disabled={
                        !dnsCredential.data?.configured ||
                        dnsCredential.data?.source === 'environment'
                      }
                      loading={savingDNSToken}
                      onClick={() => void updateDNSToken(true)}
                    >
                      {label('dns01.clearToken')}
                    </Button>
                  </Space>
                </>
              )}
            </Form>
          </Card>
          {allSetting && updateSetting && (
            <div className="domain-redirect-settings">
              <SettingListItem
                paddings="small"
                title={t('pages.settings.rootRedirectEnable')}
                description={t('pages.settings.rootRedirectDesc')}
              >
                <Switch
                  checked={allSetting.rootRedirectEnable}
                  onChange={(v) =>
                    updateSetting({
                      rootRedirectEnable: v,
                      ...(!v && rootRedirectError(allSetting.rootRedirectURL, redirectDomains)
                        ? { rootRedirectURL: '' }
                        : {}),
                    })
                  }
                />
              </SettingListItem>
              <Space orientation="vertical" style={{ width: '100%', padding: '8px 0 12px' }}>
                <Space wrap>
                  <Button
                    loading={scanningRedirect}
                    disabled={busy}
                    onClick={() => void findRedirectSite()}
                    aria-label={t('pages.settings.redirectScanAction')}
                  >
                    {t('pages.settings.redirectScanAction')}
                  </Button>
                  <Typography.Text type="secondary">
                    {t('pages.settings.redirectScanHint')}
                  </Typography.Text>
                </Space>
                {redirectScan && (
                  <>
                    <Alert
                      type={redirectScan.recommended ? 'success' : 'warning'}
                      showIcon
                      title={
                        redirectScan.recommended
                          ? t('pages.settings.redirectScanFound', { url: redirectScan.recommended })
                          : t('pages.settings.redirectScanEmpty')
                      }
                      description={t('pages.settings.redirectScanNote')}
                    />
                    <Table<RedirectSiteCheck>
                      size="small"
                      rowKey="url"
                      pagination={{ pageSize: 6, hideOnSinglePage: true }}
                      dataSource={redirectScan.sites}
                      scroll={{ x: 650 }}
                      columns={[
                        {
                          title: t('pages.settings.redirectScanWebsite'),
                          dataIndex: 'url',
                          key: 'url',
                          render: (value: string, site) => (
                            <Space>
                              <Typography.Text ellipsis style={{ maxWidth: 240 }} title={value}>
                                {value}
                              </Typography.Text>
                              {site.url === redirectScan.recommended && (
                                <Tag color="green">{t('pages.settings.redirectScanBest')}</Tag>
                              )}
                            </Space>
                          ),
                        },
                        {
                          title: t('pages.settings.redirectScanResponse'),
                          key: 'response',
                          render: (_, site) => site.status || '—',
                        },
                        {
                          title: t('pages.settings.redirectScanLatency'),
                          key: 'latency',
                          render: (_, site) => (site.available ? `${site.latencyMs} ms` : '—'),
                        },
                        {
                          title: t('pages.settings.redirectScanSNI'),
                          key: 'sni',
                          render: (_, site) =>
                            site.sniLatencyMs ? `${site.sniLatencyMs} ms` : '—',
                        },
                        {
                          title: '',
                          key: 'choose',
                          render: (_, site) => (
                            <Button
                              size="small"
                              disabled={
                                !site.available || !!rootRedirectError(site.url, redirectDomains)
                              }
                              onClick={() => updateSetting({ rootRedirectURL: site.url })}
                            >
                              {t('pages.settings.redirectScanChoose')}
                            </Button>
                          ),
                        },
                      ]}
                    />
                  </>
                )}
              </Space>
              <SettingListItem paddings="small" title={t('pages.settings.rootRedirectURL')}>
                <AutoComplete
                  status={redirectError ? 'error' : undefined}
                  maxLength={2048}
                  aria-label={t('pages.settings.rootRedirectURL')}
                  style={{ width: '100%' }}
                  value={allSetting.rootRedirectURL}
                  options={redirectSiteOptions}
                  showSearch={{
                    filterOption: (input, option) =>
                      (option?.value ?? '').toLowerCase().includes(input.toLowerCase()),
                  }}
                  defaultActiveFirstOption={false}
                  allowClear
                  onChange={(value) => updateSetting({ rootRedirectURL: value })}
                  placeholder="https://example.com/"
                />
              </SettingListItem>
              {allSetting.rootRedirectEnable && (
                <Space orientation="vertical" style={{ width: '100%', marginTop: 12 }}>
                  <Alert
                    showIcon
                    type={
                      redirectError
                        ? 'error'
                        : settingsDirty
                          ? 'warning'
                          : status?.running
                            ? 'success'
                            : 'warning'
                    }
                    title={
                      redirectError ||
                      (settingsDirty
                        ? 'Перенаправление ещё не применено'
                        : status?.running
                          ? 'Доменный шлюз работает'
                          : 'Доменный шлюз не запущен')
                    }
                    description={t('pages.settings.redirectScanRedirectDescription')}
                  />
                  {!redirectDomains.length && (
                    <Alert
                      type="warning"
                      showIcon
                      title="Сначала добавьте домен и настройте TLS. Для общего доступа нужны свободные порты 80 и 443."
                    />
                  )}
                  {status?.error && <Alert type="error" showIcon title={status.error} />}
                  {!redirectError &&
                    redirectDomains.map((name) => (
                      <Space key={name} wrap>
                        <Typography.Text code>https://{name}/</Typography.Text>
                        <span>→</span>
                        <Typography.Text ellipsis style={{ maxWidth: 320 }}>
                          {allSetting.rootRedirectURL}
                        </Typography.Text>
                      </Space>
                    ))}
                  <Typography.Text type="secondary">
                    При ошибке привязки портов или TLS изменение перенаправления откатывается.
                    Сохраните адрес панели перед включением.
                  </Typography.Text>
                </Space>
              )}
            </div>
          )}
          <Card
            loading={loading}
            title={
              <Space>
                {label('gateway')}
                <Tag color={status?.running ? 'green' : 'default'}>
                  {label(status?.running ? 'running' : 'stopped')}
                </Tag>
              </Space>
            }
          >
            <Form layout="vertical">
              <Form.Item label={label('enabled')} tooltip={label('enabledDesc')}>
                <Switch
                  aria-label={label('enabled')}
                  checked={draft.enabled}
                  onChange={(v) => update({ enabled: v })}
                />
              </Form.Item>
              <Form.Item label={label('email')} tooltip={label('emailDesc')}>
                <Input
                  aria-label={label('email')}
                  type="email"
                  value={draft.email}
                  onChange={(e) => update({ email: e.target.value })}
                />
              </Form.Item>
              <Checkbox
                checked={draft.acceptTerms}
                onChange={(e) => update({ acceptTerms: e.target.checked })}
              >
                {label('terms')}{' '}
                <a
                  href="https://letsencrypt.org/repository/"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  Let’s Encrypt
                </a>
              </Checkbox>
              <Tooltip title={label('termsHelp')}>
                <QuestionCircleOutlined
                  aria-label={label('termsHelp')}
                  tabIndex={0}
                  style={{ marginInlineStart: 8 }}
                />
              </Tooltip>
              <Collapse
                style={{ marginTop: 16 }}
                items={[
                  {
                    key: 'advanced',
                    label: label('advanced'),
                    children: (
                      <>
                        <Form.Item label={label('listen')} tooltip={label('listenHelp')}>
                          <Input
                            value={draft.listen}
                            onChange={(e) => update({ listen: e.target.value })}
                          />
                        </Form.Item>
                        <Space wrap>
                          <Form.Item label={label('httpPort')} tooltip={label('httpPortHelp')}>
                            <InputNumber
                              min={1}
                              max={65535}
                              value={draft.httpPort}
                              aria-label={label('httpPort')}
                              onChange={(v) => {
                                if (v != null) update({ httpPort: v });
                              }}
                            />
                          </Form.Item>
                          <Form.Item label={label('httpsPort')} tooltip={label('httpsPortHelp')}>
                            <InputNumber
                              min={1}
                              max={65535}
                              value={draft.httpsPort}
                              aria-label={label('httpsPort')}
                              onChange={(v) => {
                                if (v != null)
                                  update({
                                    httpsPort: v,
                                    ...(!separatePublicPort && draft.publicPort === draft.httpsPort
                                      ? { publicPort: v }
                                      : {}),
                                  });
                              }}
                            />
                          </Form.Item>
                          {(separatePublicPort || draft.publicPort !== draft.httpsPort) && (
                            <Form.Item
                              label={label('publicPort')}
                              tooltip={label('publicPortHelp')}
                            >
                              <InputNumber
                                min={1}
                                max={65535}
                                value={draft.publicPort}
                                aria-label={label('publicPort')}
                                onChange={(v) => {
                                  if (v != null) update({ publicPort: v });
                                }}
                              />
                            </Form.Item>
                          )}
                        </Space>
                        <Checkbox
                          checked={separatePublicPort || draft.publicPort !== draft.httpsPort}
                          onChange={(event) => {
                            setSeparatePublicPort(event.target.checked);
                            if (!event.target.checked) update({ publicPort: draft.httpsPort });
                          }}
                        >
                          {label('portMapping')}
                        </Checkbox>
                        <Typography.Paragraph type="secondary">
                          {label('portsHelp')}
                        </Typography.Paragraph>
                        <Checkbox
                          checked={draft.staging}
                          onChange={(e) => update({ staging: e.target.checked })}
                        >
                          {label('staging')}
                        </Checkbox>
                        <Typography.Paragraph type="secondary">
                          {label('stagingDesc')}
                        </Typography.Paragraph>
                      </>
                    ),
                  },
                ]}
              />
            </Form>
          </Card>
          {allSetting && updateSetting && (
            <Collapse
              items={[
                {
                  key: 'manual-tls',
                  label: t('pages.settings.domains.manualSettings'),
                  children: (
                    <>
                      <Link to="/settings#subscription">
                        {t('pages.settings.subscriptionAccess')}
                      </Link>
                      {(['webCertFile', 'webKeyFile', 'subCertFile', 'subKeyFile'] as const).map(
                        (key) => (
                          <SettingListItem
                            key={key}
                            paddings="small"
                            title={t(
                              `pages.settings.${({ webCertFile: 'publicKeyPath', webKeyFile: 'privateKeyPath', subCertFile: 'subCertPath', subKeyFile: 'subKeyPath' } as const)[key]}`,
                            )}
                          >
                            <Input
                              value={allSetting[key]}
                              onChange={(e) => updateSetting({ [key]: e.target.value })}
                            />
                          </SettingListItem>
                        ),
                      )}
                    </>
                  ),
                },
              ]}
            />
          )}

          <Collapse
            items={[
              {
                key: 'help',
                label: label('help'),
                children: (
                  <Space orientation="vertical" size={12}>
                    {['direct', 'cdn', 'relay', 'sub_link_only', 'sni_only'].map((key) => (
                      <div key={key}>
                        <Typography.Text strong>{label(`mode.${key}`)}</Typography.Text>
                        <Typography.Paragraph style={{ margin: 0 }}>
                          {label(`modeDesc.${key}`)}
                        </Typography.Paragraph>
                      </div>
                    ))}
                    <Typography.Paragraph>{label('transportHelp')}</Typography.Paragraph>
                    <Typography.Paragraph>{label('dnsHelp')}</Typography.Paragraph>
                  </Space>
                ),
              },
            ]}
          />
        </div>
      )}
      {status?.error && (
        <Alert type="error" showIcon title={label('gatewayError')} description={status.error} />
      )}
      <Modal
        open={editing !== undefined}
        title={label('editor')}
        onCancel={() => setEditing(undefined)}
        onOk={() =>
          saveDomain().catch((error: unknown) => {
            if (error instanceof Error) messageApi.error(error.message);
          })
        }
        forceRender
        width={580}
      >
        <Form form={form} layout="vertical" preserve={false}>
          <Form.Item
            name="name"
            label={label('name')}
            tooltip={label('nameHelp')}
            rules={[
              antdRule(DomainFormSchema.shape.name, t),
              {
                validator: async (_rule, value: string) => {
                  if (
                    draft.domains.some(
                      (d, i) => i !== editing && d.name === value?.trim().toLowerCase(),
                    )
                  )
                    throw new Error(label('duplicate'));
                },
              },
            ]}
          >
            <Input placeholder="sub.example.com" />
          </Form.Item>
          <Form.Item
            name="mode"
            label={label('modeLabel')}
            tooltip={label('modeHelp')}
            rules={[antdRule(DomainFormSchema.shape.mode, t)]}
          >
            <Select
              onChange={(value) => {
                if (
                  value === 'sub_link_only' ||
                  value === 'sni_only' ||
                  value === 'shared_sni' ||
                  value === 'cert_only'
                ) {
                  form.setFieldsValue({
                    inboundIds: [],
                    origin: '',
                    tls:
                      value === 'cert_only'
                        ? 'certmagic'
                        : value === 'shared_sni'
                          ? 'auto'
                          : form.getFieldValue('tls'),
                  });
                  if (value === 'sni_only') {
                    form.setFieldsValue({ siteCatalog: 'builtin', siteTemplate: 'notes', siteHtml: '' });
                  } else {
                    form.setFieldsValue({ siteCatalog: '', siteTemplate: '', siteHtml: '' });
                  }
                  if (value === 'sni_only' || value === 'shared_sni' || value === 'cert_only')
                    setPrimaryDomain(false);
                } else if (value !== 'relay') form.setFieldsValue({ origin: '' });
              }}
              options={[
                'direct',
                'cdn',
                'relay',
                'sub_link_only',
                'sni_only',
                'shared_sni',
                'cert_only',
              ].map((value) => ({
                value,
                label:
                  value === 'cert_only'
                    ? i18n.language.startsWith('ru')
                      ? 'Только сертификат (DNS-01)'
                      : 'Certificate only (DNS-01)'
                    : label(`mode.${value}`),
              }))}
            />
          </Form.Item>
          <Typography.Paragraph type="secondary">
            {mode === 'cert_only'
              ? i18n.language.startsWith('ru')
                ? 'Сертификат выпускается и продлевается через DNS-01 на основной ноде, без сайта и портов 80/443.'
                : 'Issues and renews a certificate via DNS-01 on this node without a website or public listeners.'
              : mode && label(`modeDesc.${mode}`)}
          </Typography.Paragraph>
          {mode === 'sni_only' && (
            <>
              <Form.Item
                name="siteCatalog"
                label={
                  i18n.language.startsWith('ru') ? 'Каталог сайта-заглушки' : 'Cover site catalog'
                }
              >
                <Select
                  options={[
                    ...Object.entries(TelemtWebCatalogs).map(([value, entry]) => ({
                      value,
                      label: i18n.language.startsWith('ru') ? entry.ru : entry.en,
                    })),
                    { value: 'custom', label: i18n.language.startsWith('ru') ? 'Свой HTML' : 'Custom HTML' },
                  ]}
                  onChange={(value: 'builtin' | 'mtproxyl' | 'custom') => {
                    form.setFieldValue('siteTemplate', value === 'custom' ? 'html' : value === 'builtin' ? 'notes' : 'stub');
                    if (value === 'custom' && !form.getFieldValue('siteHtml')) {
                      form.setFieldValue('siteHtml', starterCoverSiteHTML);
                    }
                  }
                />
              </Form.Item>
              <Form.Item
                name="siteTemplate"
                label={i18n.language.startsWith('ru') ? 'Какой сайт создать' : 'Website template'}
                rules={[{ required: true }]}
              >
                {siteCatalog === 'custom' ? (
                  <Select disabled options={[{ value: 'html', label: i18n.language.startsWith('ru') ? 'Моя HTML-страница' : 'My HTML page' }]} />
                ) : (
                  <Select
                    options={TelemtWebCatalogs[
                      siteCatalog === 'mtproxyl' ? 'mtproxyl' : 'builtin'
                    ].templates.map((entry) => ({
                      value: entry.id,
                      label: i18n.language.startsWith('ru') ? entry.ru : entry.en,
                      title: i18n.language.startsWith('ru')
                        ? entry.descriptionRu
                        : entry.descriptionEn,
                    }))}
                  />
                )}
              </Form.Item>
              {siteCatalog === 'custom' && (
                <Form.Item
                  name="siteHtml"
                  rules={[{
                    validator: (_, value: string) => coverSiteHTMLValidation(value || '')
                      ? Promise.reject(new Error(i18n.language.startsWith('ru') ? 'Укажите полный HTML до 256 КБ' : 'Enter a full HTML document up to 256 KiB'))
                      : Promise.resolve(),
                  }]}
                >
                  <Form.Item noStyle shouldUpdate={(previous, current) => previous.siteHtml !== current.siteHtml}>
                    {() => (
                      <CoverSiteHTMLEditor
                        value={form.getFieldValue('siteHtml') || ''}
                        onChange={(value) => form.setFieldValue('siteHtml', value)}
                      />
                    )}
                  </Form.Item>
                </Form.Item>
              )}
            </>
          )}
          {mode === 'relay' && (
            <Form.Item
              name="origin"
              label={label('origin')}
              tooltip={label('originDesc')}
              rules={[{ required: true, message: label('originRequired') }]}
              extra={label('originDesc')}
            >
              <Input placeholder="https://origin.example.com" />
            </Form.Item>
          )}
          {mode !== 'sub_link_only' &&
            mode !== 'sni_only' &&
            mode !== 'shared_sni' &&
            mode !== 'cert_only' && (
              <>
                <Form.Item
                  name="inboundIds"
                  label={label('inbounds')}
                  tooltip={label('inboundsDesc')}
                  extra={label('inboundsDesc')}
                >
                  <Select
                    mode="multiple"
                    options={compatible.map((o) => ({
                      value: o.id,
                      label: `${o.remark} · ${o.network} · ${o.port}`,
                    }))}
                  />
                </Form.Item>
              </>
            )}
          {mode !== 'sni_only' && mode !== 'shared_sni' && mode !== 'cert_only' && (
            <Form.Item
              label={label('useForSubscription')}
              tooltip={label('useForSubscriptionHelp')}
            >
              <Checkbox
                checked={primaryDomain}
                onChange={(event) => setPrimaryDomain(event.target.checked)}
              >
                {label('useForSubscription')}
              </Checkbox>
            </Form.Item>
          )}
          <Collapse
            size="small"
            style={{ marginTop: 12 }}
            activeKey={advancedEditorOpen ? ['domain-advanced'] : []}
            onChange={(keys) =>
              setAdvancedEditorOpen(
                Array.isArray(keys) ? keys.includes('domain-advanced') : keys === 'domain-advanced',
              )
            }
            items={[
              {
                key: 'domain-advanced',
                label: label('workspace.editorAdvanced'),
                forceRender: true,
                children: (
                  <>
                    <Form.Item
                      name="enabled"
                      label={label('domainEnabled')}
                      tooltip={label('domainEnabledHelp')}
                      valuePropName="checked"
                    >
                      <Switch />
                    </Form.Item>
                    {mode !== 'shared_sni' && mode !== 'cert_only' && (
                      <>
                        <Form.Item
                          name="tls"
                          label={label('tlsLabel')}
                          tooltip={label('tlsHelp')}
                          rules={[antdRule(DomainFormSchema.shape.tls, t)]}
                        >
                          <Select
                            options={['auto', 'certmagic', 'existing', 'manual'].map((value) => ({
                              value,
                              label: label(`tls.${value}`),
                            }))}
                          />
                        </Form.Item>
                        <Typography.Paragraph type="secondary">
                          {label(
                            tls === 'manual'
                              ? 'manualDesc'
                              : tls === 'certmagic'
                                ? 'certmagicDesc'
                                : tls === 'existing'
                                  ? 'existingTLSDesc'
                                  : 'autoDesc',
                          )}
                        </Typography.Paragraph>
                      </>
                    )}
                    {mode !== 'shared_sni' && mode !== 'cert_only' && tls === 'manual' && (
                      <>
                        <Form.Item
                          name="certFile"
                          label={label('certFile')}
                          tooltip={label('certFileHelp')}
                          rules={[{ required: true }]}
                        >
                          <Input placeholder="/etc/ssl/fullchain.pem" />
                        </Form.Item>
                        <Form.Item
                          name="keyFile"
                          label={label('keyFile')}
                          tooltip={label('keyFileHelp')}
                          rules={[{ required: true }]}
                        >
                          <Input placeholder="/etc/ssl/privkey.pem" />
                        </Form.Item>
                      </>
                    )}
                  </>
                ),
              },
            ]}
          />
          <Typography.Paragraph type="secondary" style={{ marginTop: 12, marginBottom: 0 }}>
            {label('workspace.editorSaveHint')}
          </Typography.Paragraph>
        </Form>
      </Modal>
    </Space>
  );
}
