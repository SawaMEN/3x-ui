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
  const [error, setError] = useState('');
  const [pending, setPending] = useState('');
  const fields = localCertificateFields(protocol, security);
  const certField = fields?.cert;
  const keyField = fields?.key;
  const autoTLSField = fields?.autoTLS;
  const local = nodeId == null || nodeId === 0;
  const certificate = nodeCertificateForSni(sites, sni, nodeId);
  const normalized = sni.trim().toLowerCase().replace(/\.$/, '');
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

  if (!local || !fields || !validSni || disabled) return null;

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
