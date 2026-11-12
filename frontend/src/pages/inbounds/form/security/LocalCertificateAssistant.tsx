import { useEffect, useMemo, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { useFormContext, useWatch } from 'react-hook-form';
import { Alert, Button, Checkbox, Input, Modal, Space, Typography } from 'antd';
import { useManagedSniDomains } from '@/api/queries/useManagedSniDomains';
import { HttpUtil } from '@/utils';
import MainNodeDNS01Setup, { useMainNodeDNS01Readiness } from '@/components/MainNodeDNS01Setup';
import {
  localCertificateFields,
  mayReplaceCertificatePair,
  nodeCertificateForSni,
} from './local-certificate';

type IssueResult = { domain: string; state: string; certFile?: string; keyFile?: string };
type ResolvedCertificate = {
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
  const [transferring, setTransferring] = useState(false);
  const [transferred, setTransferred] = useState('');
  const [error, setError] = useState('');
  const [pending, setPending] = useState('');
  const fields = localCertificateFields(protocol, security);
  const certField = fields?.cert;
  const keyField = fields?.key;
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

  // Remote inventory deliberately hides filesystem paths. Resolve only the
  // currently selected (node, SNI), through a scoped master-to-node API.
  const remoteLookup = useQuery({
    queryKey: ['tls-node-certificate', nodeId ?? 0, normalized, rsaOnly],
    enabled: (!local || rsaOnly) && !!fields && validSni && !disabled,
    staleTime: 60_000,
    retry: false,
    queryFn: async (): Promise<ResolvedCertificate | null> => {
      const response = await HttpUtil.post(
        '/panel/api/domains/certificates/resolve',
        { nodeId: nodeId ?? 0, domain: normalized, rsaOnly },
        { headers: { 'Content-Type': 'application/json' }, silent: true },
      );
      if (!response?.success) return null;
      const found = response.obj as ResolvedCertificate;
      if (
        found?.domain !== normalized ||
        (found?.nodeId ?? 0) !== (nodeId ?? 0) ||
        !found.certFile ||
        !found.keyFile
      )
        return null;
      return found;
    },
  });
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
        : (remoteLookup.data ?? null),
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
      // TlsForm handles locally discovered Xray pairs through its field array.
      if (local) return;
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

  const transfer = async () => {
    if (local || !nodeId || transferring || !mainCertificate || !certField || !keyField) return;
    if (
      rsaOnly &&
      (String(getValues('settings.certificatePEM') || '') ||
        String(getValues('settings.privateKeyPEM') || ''))
    ) {
      setError(
        t(
          'Сначала очистите встроенный PEM-сертификат FPTN.',
          'Clear the embedded FPTN PEM certificate before transferring.',
        ),
      );
      return;
    }
    setError('');
    setTransferred('');
    const currentCert = String(getValues(certField) || '');
    const currentKey = String(getValues(keyField) || '');
    if (!mayReplaceCertificatePair(currentCert, currentKey, sites, nodeId)) {
      setError(
        t(
          'Уже заданы пользовательские файлы сертификата. Очистите их перед передачей.',
          'Custom certificate paths are already set. Clear them before transferring.',
        ),
      );
      return;
    }
    if (certField.startsWith('streamSettings.')) {
      const rows = (getValues('streamSettings.tlsSettings.certificates') || []) as Array<{
        certificate?: string[];
        key?: string[];
        [key: string]: unknown;
      }>;
      if (rows[0]?.certificate?.length || rows[0]?.key?.length) {
        setError(
          t(
            'В форме есть встроенный PEM-сертификат. Удалите его перед передачей.',
            'An embedded PEM certificate is configured. Remove it before transferring.',
          ),
        );
        return;
      }
    }
    setTransferring(true);
    try {
      const response = await HttpUtil.post(
        '/panel/api/domains/certificates/distribute',
        { nodeId, domain: normalized, rsaOnly },
        { headers: { 'Content-Type': 'application/json' }, silent: true },
      );
      if (!response?.success) throw new Error(response?.msg || 'Certificate transfer failed');
      const result = response.obj as {
        domain: string;
        certFile: string;
        keyFile: string;
        expiresAt: number;
      };
      if (result.domain !== normalized || !result.certFile || !result.keyFile) {
        throw new Error('Incomplete certificate installation response');
      }
      const opts = { shouldDirty: true, shouldValidate: true, shouldTouch: true };
      if (certField.startsWith('streamSettings.')) {
        const rows = (getValues('streamSettings.tlsSettings.certificates') || []) as Array<
          Record<string, unknown>
        >;
        const row = {
          ...(rows[0] || {}),
          useFile: true,
          certificateFile: result.certFile,
          keyFile: result.keyFile,
          certificate: [],
          key: [],
          ocspStapling: 0,
          oneTimeLoading: false,
          usage: 'encipherment',
          buildChain: false,
        };
        setValue('streamSettings.tlsSettings.certificates', [row, ...rows.slice(1)], opts);
      } else {
        setValue(certField, result.certFile, opts);
        setValue(keyField, result.keyFile, opts);
        if (autoTLSField) setValue(autoTLSField, false, opts);
      }
      lastAutomatic.current = { ...result, nodeId };
      await queryClient.invalidateQueries({
        queryKey: ['tls-node-certificate', nodeId, normalized, rsaOnly],
      });
      setTransferred(
        t(
          'Сертификат установлен на выбранной ноде. Сохраните подключение.',
          'Certificate installed on the selected node. Save the inbound.',
        ),
      );
    } catch (e) {
      setError(String(e));
    } finally {
      setTransferring(false);
    }
  };

  const issue = async () => {
    if (issuing || !email.trim() || !consent) return;
    setIssuing(true);
    setError('');
    try {
      const response = await HttpUtil.post(
        '/panel/api/manager/domains/local-certificate',
        {
          domain: normalized,
          email: email.trim(),
          acceptTerms: true,
        },
        { headers: { 'Content-Type': 'application/json' }, silent: true },
      );
      if (!response?.success) throw new Error(response?.msg || 'Certificate issuance failed');
      const result = response.obj as IssueResult;
      setPending(result.state === 'valid' ? '' : result.state || 'pending');
      await refetch();
      setOpen(false);
    } catch (e) {
      setError(String(e));
    } finally {
      setIssuing(false);
    }
  };

  const issuanceModal = (
    <Modal
      open={open}
      title={t('TLS-сертификат на основной ноде', 'TLS certificate on the main node')}
      onCancel={() => setOpen(false)}
      onOk={() => void issue()}
      okText={t('Выпустить через DNS-01', 'Issue via DNS-01')}
      confirmLoading={issuing}
      okButtonProps={{ disabled: !email.trim() || !consent || !dns01.ready || issuing }}
    >
      <Space orientation="vertical" style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          description={t(
            'Используется DNS-01 основной ноды. Сертификат и ключ будут храниться на ней. Если DNS-01 ещё не настроен, нажмите «Настроить DNS-01» ниже.',
            'The certificate and key are issued and stored on the main node. If DNS-01 is not configured yet, use Configure DNS-01 below.',
          )}
        />
        <MainNodeDNS01Setup
          domain={normalized}
          onConfigured={() => {
            if (email.trim() && consent) void issue();
          }}
        />
        {!dns01.ready && !dns01.checking && (
          <Alert
            type="warning"
            showIcon
            description={t(
              'Сначала сохраните DNS-01 для выбранного домена. После настройки с заполненным email и согласием выпуск запустится автоматически.',
              'Set up DNS-01 for this hostname first. Once configured, issuance starts automatically if email and consent are supplied.',
            )}
          />
        )}
        {dns01.credential.isError && (
          <Alert
            type="error"
            showIcon
            title={t(
              'Не удалось проверить DNS-токен. Повторите проверку выше.',
              'Cannot verify the DNS token. Retry above.',
            )}
          />
        )}
        <Typography.Text>{normalized}</Typography.Text>
        <Input
          type="email"
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
  );

  if (!local) {
    return (
      <div style={{ marginTop: 6 }}>
        <Space wrap>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {t(
              'Обычный TLS требует локальный сертификат и закрытый ключ на выбранной ноде.',
              'Standard TLS requires a local certificate and private key on the selected node.',
            )}
          </Typography.Text>
          <Button
            size="small"
            loading={transferring}
            disabled={!mainCertificate || !nodeId}
            onClick={() => void transfer()}
          >
            {t('Передать сертификат с основной ноды', 'Transfer certificate from main node')}
          </Button>
        </Space>
        {!mainCertificate && (
          <Alert
            type="warning"
            showIcon
            description={
              <Space orientation="vertical">
                <Typography.Text>
                  {t(
                    'Сначала выпустите сертификат для управляемого домена на основной ноде через DNS-01.',
                    'First issue a certificate for your controlled domain on the main node via DNS-01.',
                  )}
                </Typography.Text>
                <Button size="small" onClick={() => setOpen(true)}>
                  {t('Выпустить сертификат (DNS-01)', 'Issue certificate (DNS-01)')}
                </Button>
                {pending && (
                  <Button size="small" onClick={() => void refetch()}>
                    {t('Проверить выпуск', 'Check issuance')} ({pending})
                  </Button>
                )}
              </Space>
            }
          />
        )}
        {issuanceModal}
        {transferred && <Alert type="success" showIcon title={transferred} />}
        {error && <Alert type="error" showIcon title={error} />}
      </div>
    );
  }

  return (
    <div style={{ marginTop: 6 }}>
      {certificate ? (
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          TLS: {certificate.certFile} · {certificate.keyFile}
        </Typography.Text>
      ) : (
        <Space wrap>
          <Typography.Text type="warning" style={{ fontSize: 12 }}>
            {t(
              'Нет действующего сертификата на основной ноде:',
              'No valid certificate on the main node:',
            )}{' '}
            {normalized}
          </Typography.Text>
          <Button size="small" onClick={() => setOpen(true)}>
            {t('Выпустить сертификат (DNS-01)', 'Issue certificate (DNS-01)')}
          </Button>
          {pending && (
            <Button size="small" onClick={() => void refetch()}>
              {t('Проверить выпуск', 'Check issuance')} ({pending})
            </Button>
          )}
        </Space>
      )}
      {issuanceModal}
    </div>
  );
}
