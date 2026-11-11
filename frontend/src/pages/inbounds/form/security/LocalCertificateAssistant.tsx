import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useFormContext, useWatch } from 'react-hook-form';
import { Alert, Button, Checkbox, Input, Modal, Space, Typography } from 'antd';
import { useManagedSniDomains } from '@/api/queries/useManagedSniDomains';
import { HttpUtil } from '@/utils';
import {
  localCertificateFields,
  mayReplaceCertificatePair,
  nodeCertificateForSni,
} from './local-certificate';

type IssueResult = { domain: string; state: string; certFile?: string; keyFile?: string };

export default function LocalCertificateAssistant({
  sni,
  disabled,
}: {
  sni: string;
  disabled?: boolean;
}) {
  const { control, getValues, setValue } = useFormContext();
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
  const certificate = nodeCertificateForSni(sites, sni, nodeId);
  const normalized = sni.trim().toLowerCase().replace(/\.$/, '');
  const mainCertificate = sites.find((site) =>
    site.name.trim().toLowerCase().replace(/\.$/, '') === normalized &&
    site.source !== 'remote' && site.source !== 'distributed' &&
    !!site.certFile && !!site.keyFile &&
    (!site.expiresAt || site.expiresAt > Date.now() / 1000),
  );
  const validSni =
    !/^\d{1,3}(?:\.\d{1,3}){3}$/.test(normalized) &&
    /^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/.test(normalized);

  // Reuse only a verified certificate on the same machine. Preserve manual paths.
  // Xray stream TLS is managed by TlsForm's useFieldArray and its own callback.
  useEffect(() => {
    if (
      !certField ||
      !keyField ||
      !certificate ||
      !local ||
      certField.startsWith('streamSettings.')
    )
      return;
    const currentCert = String(getValues(certField) || '');
    const currentKey = String(getValues(keyField) || '');
    if (!mayReplaceCertificatePair(currentCert, currentKey, sites, nodeId)) return;
    if (currentCert !== certificate.certFile) {
      setValue(certField, certificate.certFile, { shouldDirty: true, shouldValidate: true });
    }
    if (currentKey !== certificate.keyFile) {
      setValue(keyField, certificate.keyFile, { shouldDirty: true, shouldValidate: true });
    }
    if (autoTLSField) setValue(autoTLSField, false, { shouldDirty: true });
  }, [autoTLSField, certField, certificate, getValues, keyField, local, nodeId, setValue, sites]);

  if (!fields || !validSni || disabled) return null;

  const transfer = async () => {
    if (local || !nodeId || transferring || !mainCertificate || !certField || !keyField) return;
    setError('');
    setTransferred('');
    const currentCert = String(getValues(certField) || '');
    const currentKey = String(getValues(keyField) || '');
    if (!mayReplaceCertificatePair(currentCert, currentKey, sites, nodeId)) {
      setError(t(
        'Уже заданы пользовательские файлы сертификата. Очистите их перед передачей.',
        'Custom certificate paths are already set. Clear them before transferring.',
      ));
      return;
    }
    if (certField.startsWith('streamSettings.')) {
      const rows = (getValues('streamSettings.tlsSettings.certificates') || []) as Array<{
        certificate?: string[];
        key?: string[];
        [key: string]: unknown;
      }>;
      if (rows[0]?.certificate?.length || rows[0]?.key?.length) {
        setError(t(
          'В форме есть встроенный PEM-сертификат. Удалите его перед передачей.',
          'An embedded PEM certificate is configured. Remove it before transferring.',
        ));
        return;
      }
    }
    setTransferring(true);
    try {
      const response = await HttpUtil.post(
        '/panel/api/domains/certificates/distribute',
        { nodeId, domain: normalized },
        { headers: { 'Content-Type': 'application/json' }, silent: true },
      );
      if (!response?.success) throw new Error(response?.msg || 'Certificate transfer failed');
      const result = response.obj as {
        domain: string; certFile: string; keyFile: string; expiresAt: number;
      };
      if (result.domain !== normalized || !result.certFile || !result.keyFile) {
        throw new Error('Incomplete certificate installation response');
      }
      const opts = { shouldDirty: true, shouldValidate: true, shouldTouch: true };
      if (certField.startsWith('streamSettings.')) {
        const rows = (getValues('streamSettings.tlsSettings.certificates') || []) as Array<Record<string, unknown>>;
        const row = {
          ...(rows[0] || {}),
          useFile: true, certificateFile: result.certFile, keyFile: result.keyFile,
          certificate: [], key: [], ocspStapling: 0, oneTimeLoading: false,
          usage: 'encipherment', buildChain: false,
        };
        setValue('streamSettings.tlsSettings.certificates', [row, ...rows.slice(1)], opts);
      } else {
        setValue(certField, result.certFile, opts);
        setValue(keyField, result.keyFile, opts);
        if (autoTLSField) setValue(autoTLSField, false, opts);
      }
      setTransferred(t(
        'Сертификат установлен на выбранной ноде. Сохраните подключение.',
        'Certificate installed on the selected node. Save the inbound.',
      ));
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
            description={t(
              'Сначала выпустите сертификат для управляемого домена на основной ноде через DNS-01.',
              'First issue a certificate for your controlled domain on the main node via DNS-01.',
            )}
          />
        )}
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
      <Modal
        open={open}
        title={t('TLS-сертификат на основной ноде', 'TLS certificate on the main node')}
        onCancel={() => setOpen(false)}
        onOk={() => void issue()}
        okText={t('Выпустить через DNS-01', 'Issue via DNS-01')}
        confirmLoading={issuing}
        okButtonProps={{ disabled: !email.trim() || !consent }}
      >
        <Space orientation="vertical" style={{ width: '100%' }}>
          <Alert
            type="info"
            showIcon
            description={t(
              'Используется DNS-01 основной ноды. Сертификат и ключ будут храниться локально. Настройте DNS-01 в разделе «Домены и TLS».',
              'The certificate and key are issued and stored on the main node. Configure DNS-01 under Domains & TLS.',
            )}
          />
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
    </div>
  );
}
