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
  Drawer,
  Tabs,
  Checkbox,
  Form,
  Input,
  InputNumber,
  List,
  Modal,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import {
  QuestionCircleOutlined,
  GlobalOutlined,
  SafetyCertificateOutlined,
  ApartmentOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import { HttpUtil } from '@/utils';
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
import DomainServiceMap from './DomainServiceMap';
import DomainHostList from './DomainHostList';
import DomainCreateDialog from './DomainCreateDialog';
import DomainEditor from './DomainEditor';
import DomainSniView from './DomainSniView';
import CertificateInventoryPanel from './CertificateInventoryPanel';
import NodeCertificateSharing from './NodeCertificateSharing';
import PanelDomainCertificate from './PanelDomainCertificate';
import type { PanelDomainPatch } from './PanelDomainCertificate';
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

type DomainSection = 'domains' | 'certificates' | 'sni';

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
  onPanelDomainSwitch,
}: {
  onPanelDomainSwitch?: (patch: PanelDomainPatch) => void;
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
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsTab, setSettingsTab] = useState('general');
  const [createOpen, setCreateOpen] = useState(false);
  const [panelOpen, setPanelOpen] = useState(false);
  const [preflightOpen, setPreflightOpen] = useState(false);
  const [expanded, setExpanded] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [preflighting, setPreflighting] = useState(false);
  const [preflightReport, setPreflightReport] = useState<DomainPreflightReport>();
  const [preflightDraft, setPreflightDraft] = useState('');
  const [scanningRedirect, setScanningRedirect] = useState(false);
  const [redirectScan, setRedirectScan] = useState<RedirectSiteScan>();
  const [separatePublicPort, setSeparatePublicPort] = useState(false);
  const [editing, setEditing] = useState<number>();
  const [sharingCertificate, setSharingCertificate] = useState(false);
  const [primaryDomain, setPrimaryDomain] = useState(false);
  const [editorTab, setEditorTab] = useState('details');
  const [dnsToken, setDnsToken] = useState('');
  const [savingDNSToken, setSavingDNSToken] = useState(false);
  const dnsCredential = useQuery({
    queryKey: ['settings', 'domains', 'dns-credential', draft.dns01Provider || 'cloudflare'],
    enabled: settingsOpen && settingsTab === 'dns' && !!draft.dns01Provider,
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
      setPreflightOpen(true);
      if (!report.ready) messageApi.warning(label('preflight.blocked'));
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('somethingWentWrong'));
    } finally {
      setPreflighting(false);
    }
  }

  async function apply() {
    if (busy || preflighting || !initialized) return;
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
          setPreflightOpen(true);
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
      let detail =
        e instanceof Error ? t(e.message, { defaultValue: e.message }) : t('somethingWentWrong');
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
    if (preset === 'certificate') {
      form.setFieldsValue({ mode: 'cert_only', tls: 'certmagic' });
      setPrimaryDomain(false);
    }
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
    setEditorTab(index >= 0 && draft.domains[index]?.tls !== 'auto' ? 'tls' : 'details');
    setEditing(index);
  }
  async function saveDomain() {
    const value = await form.validateFields().catch((error: unknown) => {
      const field = (error as { errorFields?: { name: (string | number)[] }[] }).errorFields?.[0]
        ?.name[0];
      if (field) {
        setEditorTab(
          ['tls', 'certFile', 'keyFile', 'enabled'].includes(String(field))
            ? 'tls'
            : ['siteCatalog', 'siteTemplate', 'siteHtml'].includes(String(field))
              ? 'site'
              : 'details',
        );
      }
      throw error;
    });
    if (value.mode === 'sni_only' && telemtSites.some((site) => site.name === value.name)) {
      messageApi.info(label('telemtAlreadyExists'));
      setEditing(undefined);
      return;
    }
    const old = editing != null && editing >= 0 ? draft.domains[editing] : undefined;
    const domain = DomainFormSchema.parse({
      ...defaultDomain,
      ...value,
      // Hidden form fields are unregistered (preserve=false); certificate-only
      // domains must retain CertMagic instead of falling back to gateway TLS.
      tls:
        value.mode === 'cert_only' ? 'certmagic' : value.mode === 'shared_sni' ? 'auto' : value.tls,
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
      siteHtml:
        value.mode === 'sni_only' && value.siteCatalog === 'custom' ? value.siteHtml || '' : '',
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

  function removeDomain(name: string) {
    Modal.confirm({
      title: label('removeConfirm'),
      okText: label('remove'),
      okButtonProps: { danger: true },
      onOk: () =>
        update({
          domains: draft.domains.filter((d) => d.name !== name),
          subscriptionDomain: draft.subscriptionDomain === name ? '' : draft.subscriptionDomain,
        }),
    });
  }
  const saveControls =
    dirty || settingsDirty ? (
      <div className="domain-manager-savebar" role="region" aria-label={label('workspace.changes')}>
        <div>
          <Typography.Text strong>{label('unsaved')}</Typography.Text>
          <Typography.Text type="secondary">{label('redesign.applyHint')}</Typography.Text>
        </div>
        <Button
          type="primary"
          loading={busy || preflighting}
          disabled={!initialized || !!redirectError || busy || preflighting}
          onClick={() => void apply()}
        >
          {label('apply')}
        </Button>
      </div>
    ) : null;
  const navigation = [
    {
      key: 'domains',
      icon: <GlobalOutlined />,
      title: label('workspace.section.domains'),
      count: draft.domains.length,
    },
    {
      key: 'certificates',
      icon: <SafetyCertificateOutlined />,
      title: label('workspace.section.certificates'),
      count: status?.certificates.length ?? 0,
    },
    {
      key: 'sni',
      icon: <ApartmentOutlined />,
      title: label('workspace.section.sni'),
      count: managedSniDomains.length,
    },
  ] as const;
  return (
    <div className="domain-hub domain-manager">
      {holder}
      <DomainOverview
        status={status}
        loading={loading}
        busy={busy || preflighting}
        dirty={dirty}
        auditing={auditing}
        onAdd={() => setCreateOpen(true)}
        onRefresh={() => void load()}
        onPreflight={() => void checkReadiness()}
        onAudit={() => void auditDomains()}
      />
      <DomainServiceMap
        settings={allSetting}
        draft={draft}
        sites={managedSniDomains}
        onPanel={() => {
          if (onPanelDomainSwitch) setPanelOpen(true);
          else {
            setSettingsTab('manual');
            setSettingsOpen(true);
          }
        }}
        onSubscription={() => {
          if (draft.subscriptionDomain) {
            setDomainSearch(draft.subscriptionDomain);
            setDomainFilter('all');
            setSection('domains');
          } else openPreset('subscription');
        }}
        onNodes={() => setSection('sni')}
      />
      {statusQuery.error && (
        <Alert
          type="error"
          showIcon
          title={label('gatewayError')}
          description={statusQuery.error.message}
        />
      )}
      {status?.error && (
        <Alert type="error" showIcon title={label('gatewayError')} description={status.error} />
      )}
      {draft.enabled &&
        draft.domains.some(
          (d) =>
            d.enabled && d.mode !== 'shared_sni' && (d.tls === 'auto' || d.tls === 'certmagic'),
        ) &&
        (!draft.email || !draft.acceptTerms) && (
          <Alert
            type="warning"
            showIcon
            title={label('workspace.autoTLSSetup')}
            action={
              <Button
                onClick={() => {
                  setSettingsTab('general');
                  setSettingsOpen(true);
                }}
              >
                {label('workspace.configureTLS')}
              </Button>
            }
          />
        )}
      {!draft.dns01Provider && draft.domains.some((d) => d.enabled && d.mode === 'cert_only') && (
        <Alert
          type="warning"
          showIcon
          title={label('workspace.dnsRequired')}
          action={
            <Button
              onClick={() => {
                setSettingsTab('dns');
                setSettingsOpen(true);
              }}
            >
              {label('workspace.configureDNS')}
            </Button>
          }
        />
      )}
      <div className="domain-hub-body">
        <nav className="domain-manager-nav" aria-label={label('workspace.navigation')}>
          {navigation.map((item) => (
            <button
              key={item.key}
              type="button"
              className={section === item.key ? 'is-active' : ''}
              aria-label={item.title}
              aria-current={section === item.key ? 'page' : undefined}
              onClick={() => setSection(item.key)}
            >
              {item.icon}
              <span>{item.title}</span>
              <small>{item.count}</small>
            </button>
          ))}
          <button
            type="button"
            aria-label={label('workspace.section.settings')}
            onClick={() => setSettingsOpen(true)}
          >
            <SettingOutlined />
            <span>{label('workspace.section.settings')}</span>
          </button>
          <div className="domain-nav-note">
            <LockStatus enabled={draft.enabled} />
            <Typography.Text type="secondary">
              {draft.listen} · {draft.httpPort} / {draft.httpsPort}
            </Typography.Text>
          </div>
        </nav>
        <main className="domain-hub-content">
          {section === 'domains' && (
            <div data-testid="domain-panel-domains">
              <div className="domain-section-heading">
                <div>
                  <Typography.Title level={4}>{label('list')}</Typography.Title>
                  <Typography.Text type="secondary">
                    {label('redesign.domainsHint')}
                  </Typography.Text>
                </div>
              </div>
              <DomainHostList
                domains={filteredDomains}
                draft={draft}
                status={status}
                sites={managedSniDomains}
                checks={checks}
                search={domainSearch}
                filter={domainFilter}
                onSearch={setDomainSearch}
                onFilter={setDomainFilter}
                onEdit={(name) => openEditor(draft.domains.findIndex((d) => d.name === name))}
                onRemove={removeDomain}
                onCheck={(name) => void check(name)}
                checking={checking}
                disabled={dirty || !initialized || auditing || busy || !!checking}
                expanded={expanded}
              />
            </div>
          )}
          {section === 'certificates' && (
            <div className="domain-manager-panel" data-testid="domain-panel-certificates">
              <div className="domain-section-heading">
                <div>
                  <Typography.Title level={4}>
                    {label('workspace.section.certificates')}
                  </Typography.Title>
                  <Typography.Text type="secondary">
                    {label('redesign.certificatesHint')}
                  </Typography.Text>
                </div>
                <Button onClick={() => openPreset('certificate')} disabled={!initialized || busy}>
                  {label('workspace.issueCertificate')}
                </Button>
              </div>
              <CertificateInventoryPanel compact onShare={() => setSharingCertificate(true)} />
            </div>
          )}
          {section === 'sni' && (
            <DomainSniView
              sites={managedSniDomains}
              pending={draft.domains.filter(
                (domain) =>
                  ['sni_only', 'shared_sni'].includes(domain.mode) &&
                  !managedSniDomains.some((site) => site.name === domain.name),
              )}
              onEdit={(name) =>
                openEditor(draft.domains.findIndex((domain) => domain.name === name))
              }
              onShare={() => setSharingCertificate(true)}
            />
          )}
          {auditing && (
            <Alert
              type="info"
              showIcon
              title={`${label('workspace.audit')}: ${auditDone} / ${auditTotal}`}
            />
          )}
        </main>
      </div>
      {!settingsOpen && saveControls}
      <Drawer
        open={settingsOpen}
        title={label('redesign.gatewaySettings')}
        size={760}
        onClose={() => setSettingsOpen(false)}
        destroyOnHidden
        footer={settingsOpen ? saveControls : null}
      >
        <Tabs
          activeKey={settingsTab}
          onChange={setSettingsTab}
          items={[
            {
              key: 'general',
              label: label('redesign.automaticTLS'),
              children: (
                <div className="domain-settings-fields">
                  {' '}
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
                  </Form>
                </div>
              ),
            },
            {
              key: 'network',
              label: label('redesign.network'),
              children: (
                <Form layout="vertical">
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
                        <Form.Item label={label('publicPort')} tooltip={label('publicPortHelp')}>
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
                </Form>
              ),
            },
            {
              key: 'dns',
              label: 'DNS-01',
              children: (
                <>
                  <Typography.Paragraph type="secondary">
                    {label('dns01.hint')}
                  </Typography.Paragraph>
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
                    {(draft.dns01Provider === 'cloudflare' ||
                      draft.dns01Provider === 'digitalocean') && (
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
                </>
              ),
            },
            ...(allSetting && updateSetting
              ? [
                  {
                    key: 'redirect',
                    label: label('redesign.redirect'),
                    children: (
                      <>
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
                                    ...(!v &&
                                    rootRedirectError(allSetting.rootRedirectURL, redirectDomains)
                                      ? { rootRedirectURL: '' }
                                      : {}),
                                  })
                                }
                              />
                            </SettingListItem>
                            <Space
                              orientation="vertical"
                              style={{ width: '100%', padding: '8px 0 12px' }}
                            >
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
                                        ? t('pages.settings.redirectScanFound', {
                                            url: redirectScan.recommended,
                                          })
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
                                            <Typography.Text
                                              ellipsis
                                              style={{ maxWidth: 240 }}
                                              title={value}
                                            >
                                              {value}
                                            </Typography.Text>
                                            {site.url === redirectScan.recommended && (
                                              <Tag color="green">
                                                {t('pages.settings.redirectScanBest')}
                                              </Tag>
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
                                        render: (_, site) =>
                                          site.available ? `${site.latencyMs} ms` : '—',
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
                                              !site.available ||
                                              !!rootRedirectError(site.url, redirectDomains)
                                            }
                                            onClick={() =>
                                              updateSetting({ rootRedirectURL: site.url })
                                            }
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
                            <SettingListItem
                              paddings="small"
                              title={t('pages.settings.rootRedirectURL')}
                            >
                              <AutoComplete
                                status={redirectError ? 'error' : undefined}
                                maxLength={2048}
                                aria-label={t('pages.settings.rootRedirectURL')}
                                style={{ width: '100%' }}
                                value={allSetting.rootRedirectURL}
                                options={redirectSiteOptions}
                                showSearch={{
                                  filterOption: (input, option) =>
                                    (option?.value ?? '')
                                      .toLowerCase()
                                      .includes(input.toLowerCase()),
                                }}
                                defaultActiveFirstOption={false}
                                allowClear
                                onChange={(value) => updateSetting({ rootRedirectURL: value })}
                                placeholder="https://example.com/"
                              />
                            </SettingListItem>
                            {allSetting.rootRedirectEnable && (
                              <Space
                                orientation="vertical"
                                style={{ width: '100%', marginTop: 12 }}
                              >
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
                                {status?.error && (
                                  <Alert type="error" showIcon title={status.error} />
                                )}
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
                                  При ошибке привязки портов или TLS изменение перенаправления
                                  откатывается. Сохраните адрес панели перед включением.
                                </Typography.Text>
                              </Space>
                            )}
                          </div>
                        )}
                      </>
                    ),
                  },
                  {
                    key: 'manual',
                    label: label('redesign.manualTLS'),
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
                ]
              : []),
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
      </Drawer>
      <Drawer
        open={panelOpen}
        title={label('panelDomain.title')}
        size={620}
        onClose={() => setPanelOpen(false)}
        destroyOnHidden
      >
        {allSetting && onPanelDomainSwitch && (
          <PanelDomainCertificate
            compact
            status={status}
            allSetting={allSetting}
            disabled={dirty || loading || busy || preflighting}
            onSwitch={onPanelDomainSwitch}
          />
        )}
      </Drawer>
      <Drawer
        open={preflightOpen}
        title={label('preflight.title')}
        size={680}
        onClose={() => setPreflightOpen(false)}
        destroyOnHidden
      >
        {' '}
        {preflightReport && preflightDraft === JSON.stringify(draft) && (
          <Card size="small">
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
      </Drawer>
      <DomainCreateDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onChoose={(preset) => {
          setCreateOpen(false);
          openPreset(preset);
        }}
      />
      <DomainEditor
        open={editing !== undefined}
        editing={editing}
        form={form}
        draft={draft}
        compatible={compatible}
        primaryDomain={primaryDomain}
        setPrimaryDomain={setPrimaryDomain}
        tab={editorTab}
        onTab={setEditorTab}
        onClose={() => setEditing(undefined)}
        onSave={() =>
          void saveDomain().catch((error: unknown) => {
            if (error instanceof Error) messageApi.error(error.message);
          })
        }
      />
      {sharingCertificate && (
        <NodeCertificateSharing onClose={() => setSharingCertificate(false)} />
      )}
    </div>
  );
}
function LockStatus({ enabled }: { enabled: boolean }) {
  const { t } = useTranslation();
  return (
    <Tag color={enabled ? 'processing' : 'default'}>
      {t(
        'pages.settings.domains.' +
          (enabled ? 'redesign.gatewayEnabled' : 'redesign.gatewayDisabled'),
      )}
    </Tag>
  );
}
