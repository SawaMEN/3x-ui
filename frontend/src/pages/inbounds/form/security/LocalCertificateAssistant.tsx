import { useEffect, useMemo, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { useFormContext, useWatch } from 'react-hook-form';
import { Alert, Button, Checkbox, Input, Modal, Space, Tooltip, Typography } from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import { useManagedSniDomains } from '@/api/queries/useManagedSniDomains';
import { HttpUtil } from '@/utils';
import MainNodeDNS01Setup, { useMainNodeDNS01Readiness } from '@/components/MainNodeDNS01Setup';
import {
  localCertificateFields,
  mayReplaceCertificatePair,
  nodeCertificateForSni,
} from './local-certificate';

type ResolvedCertificate = {
  state?: string;
  domain: string;
  certFile: string;
  keyFile: string;
  expiresAt: number;
  nodeId?: number;
};

export default function LocalCertificateAssistant({
  sni,
  disabled,
}: {
  sni: string;
  disabled?: boolean;
}) {
  const { control, getValues, setValue } = useFormContext();
  const queryClient = useQueryClient();
  const lastAutomatic = useRef<(ResolvedCertificate & { nodeId: number }) | null>(null);
  const { i18n } = useTranslation();
  const t = (ru: string, en: string) => (i18n.language.startsWith('ru') ? ru : en);
  const protocol = String(useWatch({ control, name: 'protocol' }) || '');
  const security = String(useWatch({ control, name: 'streamSettings.security' }) || '');
  const nodeId = useWatch({ control, name: 'nodeId' }) as number | null | undefined;
  const { data: sites = [], refetch } = useManagedSniDomains();
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState('');
  const [consent, setConsent] = useState(false);
  const [issuing, setIssuing] = useState(false);

  const [failure, setFailure] = useState<{ identity: string; message: string } | null>(null);

  const fields = localCertificateFields(protocol, security);
  const certField = fields?.cert;
  const keyField = fields?.key;
  const watchedCert = useWatch({ control, name: certField || '__unusedCertificate' });
  const watchedKey = useWatch({ control, name: keyField || '__unusedKey' });
  const watchedInline = useWatch({ control, name: 'streamSettings.tlsSettings.certificates.0' });
  const inlineFptnCert = useWatch({ control, name: 'settings.certificatePEM' });
  const inlineFptnKey = useWatch({ control, name: 'settings.privateKeyPEM' });

  const autoTLSField = fields?.autoTLS;
  const local = nodeId == null || nodeId === 0;
  const rsaOnly = protocol === 'fptn';
  const localCertificate = nodeCertificateForSni(sites, sni, nodeId);
  const normalized = sni.trim().toLowerCase().replace(/\.$/, '');
  const dns01 = useMainNodeDNS01Readiness(normalized);
  const mainCertificate = sites.find(
    (site) =>
      site.name.trim().toLowerCase().replace(/\.$/, '') === normalized &&
      site.source !== 'remote' &&
      site.source !== 'distributed' &&
      !!site.certFile &&
      !!site.keyFile,
  );
  const validSni =
    !/^\d{1,3}(?:\.\d{1,3}){3}$/.test(normalized) &&
    /^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/.test(normalized);

  const identity = JSON.stringify([nodeId ?? 0, normalized, protocol, security]);
  const identityRef = useRef(identity);
  useEffect(() => {
    identityRef.current = identity;
  }, [identity]);
  const error = failure?.identity === identity ? failure.message : '';
  const setError = (message: string) => setFailure({ identity, message });
  const [lookupSni, setLookupSni] = useState('');
  useEffect(() => {
    const timer = setTimeout(() => setLookupSni(normalized), 450);
    return () => clearTimeout(timer);
  }, [normalized]);
  const lookupKey = ['inbound-certificate-preparation', nodeId ?? 0, normalized, rsaOnly];
  const remoteLookup = useQuery<ResolvedCertificate | null>({
    queryKey: lookupKey,
    enabled: !!fields && validSni && lookupSni === normalized && !disabled,
    staleTime: 15_000,
    retry: false,
    refetchInterval: (query) =>
      ['pending', 'retrying'].includes(query.state.data?.state || '') ? 2_000 : false,
    queryFn: async ({ signal }): Promise<ResolvedCertificate | null> => {
      const response = await HttpUtil.get(
        '/panel/api/domains/certificates/preparation',
        { nodeId: nodeId ?? 0, domain: normalized, rsaOnly },
        { silent: true, signal },
      );
      if (!response?.success) throw new Error(response?.msg || 'Cannot check certificate status');
      const found = response.obj as ResolvedCertificate;
      if (found?.domain !== normalized || found.nodeId !== (nodeId ?? 0))
        throw new Error('Unexpected certificate response');
      if (
        found.state === 'valid' &&
        (!found.certFile || !found.keyFile || found.expiresAt <= Date.now() / 1000)
      )
        throw new Error('Invalid certificate response');
      return found;
    },
  });
  // Clear only a pair previously filled by this assistant when changing the
  // node/SNI/protocol. A late response must never attach another node's files.
  useEffect(() => {
    const previous = lastAutomatic.current;
    if (!previous || !certField || !keyField) return;
    if (
      String(getValues(certField) || '') === previous.certFile &&
      String(getValues(keyField) || '') === previous.keyFile
    ) {
      setValue(certField, '', { shouldDirty: true });
      setValue(keyField, '', { shouldDirty: true });
    }
    lastAutomatic.current = null;
  }, [identity, certField, keyField, getValues, setValue]);
  const certificate: ResolvedCertificate | null = useMemo(
    () =>
      local && !rsaOnly && localCertificate
        ? {
            domain: localCertificate.name,
            certFile: localCertificate.certFile,
            keyFile: localCertificate.keyFile,
            expiresAt: localCertificate.expiresAt || 0,
            nodeId: 0,
          }
        : remoteLookup.data?.state === 'valid'
          ? remoteLookup.data
          : null,
    [local, localCertificate, remoteLookup.data, rsaOnly],
  );
  // Reuse only a verified certificate on the chosen machine. Never replace
  // a manually entered pair; a pair filled by this assistant may rotate.
  useEffect(() => {
    if (!fields || !certField || !keyField || !certificate || !validSni || disabled) return;
    if (certificate.domain && certificate.domain !== normalized) return;
    if (
      rsaOnly &&
      (String(getValues('settings.certificatePEM') || '') ||
        String(getValues('settings.privateKeyPEM') || ''))
    )
      return;
    const pair = { certFile: certificate.certFile, keyFile: certificate.keyFile };
    if (!pair.certFile || !pair.keyFile) return;
    const savedNode = nodeId ?? 0;
    const previous = lastAutomatic.current;
    const canReplace = (cert: string, key: string) =>
      mayReplaceCertificatePair(cert, key, sites, nodeId) ||
      (previous?.certFile === cert && previous.keyFile === key);

    const opts = { shouldDirty: true, shouldValidate: true };
    if (certField.startsWith('streamSettings.')) {
      const rows = (getValues('streamSettings.tlsSettings.certificates') || []) as Array<{
        useFile?: boolean;
        certificate?: string[];
        key?: string[];
        certificateFile?: string;
        keyFile?: string;
        [key: string]: unknown;
      }>;
      const first = rows[0];
      if (first && !first.useFile && (first.certificate?.length || first.key?.length)) return;
      if (!canReplace(first?.certificateFile || '', first?.keyFile || '')) return;
      if (first?.certificateFile !== pair.certFile || first?.keyFile !== pair.keyFile) {
        setValue(
          'streamSettings.tlsSettings.certificates',
          [
            {
              ...(first || {}),
              useFile: true,
              certificateFile: pair.certFile,
              keyFile: pair.keyFile,
              certificate: [],
              key: [],
              ocspStapling: first?.ocspStapling ?? 0,
              oneTimeLoading: false,
              usage: first?.usage ?? 'encipherment',
              buildChain: first?.buildChain ?? false,
            },
            ...rows.slice(1),
          ],
          opts,
        );
      }
    } else {
      const currentCert = String(getValues(certField) || '');
      const currentKey = String(getValues(keyField) || '');
      if (!canReplace(currentCert, currentKey)) return;
      if (currentCert !== pair.certFile) setValue(certField, pair.certFile, opts);
      if (currentKey !== pair.keyFile) setValue(keyField, pair.keyFile, opts);
      if (autoTLSField && getValues(autoTLSField) !== false) setValue(autoTLSField, false, opts);
    }
    lastAutomatic.current = {
      ...pair,
      domain: normalized,
      expiresAt: certificate.expiresAt || 0,
      nodeId: savedNode,
    };
  }, [
    autoTLSField,
    certField,
    certificate,
    disabled,
    fields,
    getValues,
    keyField,
    local,
    nodeId,
    normalized,
    rsaOnly,
    setValue,
    sites,
    validSni,
  ]);

  if (!fields || !validSni || disabled) return null;

  const state = remoteLookup.data?.state;
  const pending = state === 'pending' || state === 'retrying';
  const available = state === 'available' || !!mainCertificate;
  const customPair =
    !mayReplaceCertificatePair(
      String(watchedCert || ''),
      String(watchedKey || ''),
      sites,
      nodeId,
    ) && !(certificate?.certFile === watchedCert && certificate?.keyFile === watchedKey);
  const embedded = certField?.startsWith('streamSettings.')
    ? !watchedInline?.useFile && (watchedInline?.certificate?.length || watchedInline?.key?.length)
    : rsaOnly && (inlineFptnCert || inlineFptnKey);
  const manual = customPair || !!embedded;

  const request = async (contact?: string, accepted?: boolean) => {
    if (issuing || manual) return;
    const startedFor = identity;
    const key = [...lookupKey];
    setIssuing(true);
    setError('');
    try {
      const response = await HttpUtil.post(
        '/panel/api/domains/certificates/prepare',
        {
          nodeId: nodeId ?? 0,
          domain: normalized,
          rsaOnly,
          ...(contact ? { email: contact.trim(), acceptTerms: accepted === true } : {}),
        },
        { headers: { 'Content-Type': 'application/json' }, silent: true },
      );
      if (!response?.success) throw new Error(response?.msg || 'Certificate preparation failed');
      const result = response.obj as ResolvedCertificate;
      if (result.domain !== normalized || result.nodeId !== (nodeId ?? 0))
        throw new Error('Unexpected certificate response');
      if (
        result.state === 'valid' &&
        (!result.certFile || !result.keyFile || result.expiresAt <= Date.now() / 1000)
      )
        throw new Error('Invalid certificate response');
      if (identityRef.current !== startedFor) return;
      queryClient.setQueryData(key, result);
      setOpen(false);
      await refetch();
      await queryClient.invalidateQueries({ queryKey: key });
    } catch (e) {
      if (identityRef.current === startedFor) setError(e instanceof Error ? e.message : String(e));
    } finally {
      setIssuing(false);
    }
  };
  const start = () => {
    setError('');
    if (available || (dns01.ready && dns01.config?.email && dns01.config.acceptTerms)) {
      void request();
    } else {
      setEmail(dns01.config?.email || '');
      setConsent(dns01.config?.acceptTerms === true);
      setOpen(true);
    }
  };
  return (
    <div style={{ marginTop: 6 }}>
      <Space wrap>
        {manual ? (
          <Typography.Text type="secondary">
            {t(
              'Используются заданные вручную файлы или PEM. Очистите их для автоматической настройки.',
              'Custom certificate files or PEM are preserved. Clear them to use automatic setup.',
            )}
          </Typography.Text>
        ) : certificate ? (
          <Typography.Text type="success">
            {t('Сертификат готов для выбранной ноды', 'Certificate ready on the selected node')}:{' '}
            {normalized}
          </Typography.Text>
        ) : pending ? (
          <Typography.Text type="secondary">
            {state === 'retrying'
              ? t(
                  'Ожидаем сертификат или доступность ноды. Повторяем автоматически.',
                  'Waiting for the certificate or node. Retrying automatically.',
                )
              : t(
                  'Выпускаем и доставляем сертификат. Ожидайте — форма обновится автоматически.',
                  'Preparing and delivering the certificate. This form updates automatically.',
                )}
          </Typography.Text>
        ) : (
          <Button
            size="small"
            type="primary"
            loading={issuing}
            disabled={issuing || remoteLookup.isFetching || lookupSni !== normalized}
            onClick={start}
          >
            {available
              ? t('Получить сертификат на эту ноду', 'Use certificate on this node')
              : t('Получить сертификат (DNS-01)', 'Get certificate (DNS-01)')}
          </Button>
        )}
        <Tooltip
          title={t(
            'Сначала ищем сертификат на выбранной ноде. Если его нет, используем сертификат основной панели или выпускаем его через DNS-01 и автоматически доставляем ноде. Готовый сайт может находиться на другом сервере: сайт, A/AAAA-записи и порты 80/443 менять не нужно. Продление и доставка обновлений автоматические. Пользовательские файлы не заменяются.',
            'A matching certificate on this node is reused first. Otherwise the main panel reuses or issues one via DNS-01 and delivers it automatically, including renewals. Your existing website can stay on another server: no website, A/AAAA records or ports 80/443 need changing. Custom certificate files are preserved.',
          )}
        >
          <QuestionCircleOutlined aria-label={t('Как получить сертификат', 'Certificate help')} />
        </Tooltip>
      </Space>
      {rsaOnly && !certificate && !available && !pending && (
        <Alert
          type="info"
          showIcon
          description={t(
            'FPTN нужен RSA-сертификат. Используйте существующую RSA-пару; автоматический выпуск ECDSA для него не подходит.',
            'FPTN requires an RSA certificate. Use an existing RSA pair; automatic ECDSA issuance is incompatible.',
          )}
        />
      )}
      {remoteLookup.error && (
        <Typography.Text type="warning">{String(remoteLookup.error)} </Typography.Text>
      )}
      {error && <Alert type="error" showIcon title={error} />}
      <Modal
        open={open}
        title={t('Сертификат для подключения', 'Certificate for this inbound')}
        onCancel={() => setOpen(false)}
        onOk={() => void request(email, consent)}
        okText={t('Выпустить и подключить', 'Issue and connect')}
        confirmLoading={issuing}
        okButtonProps={{
          disabled: !email.trim() || !consent || !dns01.ready || issuing || rsaOnly,
        }}
      >
        <Space orientation="vertical" style={{ width: '100%' }}>
          <Typography.Paragraph>
            {t('Для', 'For')} <strong>{normalized}</strong>.{' '}
            {t(
              local
                ? 'Сертификат будет выпущен через DNS-01 на основном сервере.'
                : 'Выпуск через DNS-01 и доставка выбранной ноде выполнятся автоматически. Не нужно создавать новый сайт.',
              local
                ? 'The main server will issue the certificate using DNS-01.'
                : 'DNS-01 issuance and delivery to this node are automatic. No new website is needed.',
            )}
          </Typography.Paragraph>
          <MainNodeDNS01Setup
            domain={normalized}
            onConfigured={() => {
              if (email.trim() && consent) void request(email, consent);
            }}
          />
          <Input
            type="email"
            aria-label="ACME email"
            maxLength={254}
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="Email для Let's Encrypt"
          />
          <Checkbox checked={consent} onChange={(e) => setConsent(e.target.checked)}>
            {t(
              'Я владею доменом и принимаю условия Let’s Encrypt.',
              'I control this domain and accept the Let’s Encrypt terms.',
            )}
          </Checkbox>
          {error && <Alert showIcon type="error" title={error} />}
        </Space>
      </Modal>
    </div>
  );
}
