import { useEffect, useState } from 'react';
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

export default function LocalCertificateAssistant({ sni, disabled }: { sni: string; disabled?: boolean }) {
  const { control, getValues, setValue } = useFormContext();
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
  const local = nodeId == null || nodeId === 0;
  const certificate = nodeCertificateForSni(sites, sni, nodeId);
  const normalized = sni.trim().toLowerCase().replace(/\.$/, '');
  const validSni = /^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/.test(normalized);

  // Reuse only a verified certificate on the same machine. Preserve manual paths.
  // Xray stream TLS is managed by TlsForm's useFieldArray and its own callback.
  useEffect(() => {
    if (!fields || !certificate || !local || fields.cert.startsWith('streamSettings.')) return;
    const currentCert = String(getValues(fields.cert) || '');
    const currentKey = String(getValues(fields.key) || '');
    if (!mayReplaceCertificatePair(currentCert, currentKey, sites, nodeId)) return;
    if (currentCert !== certificate.certFile) {
      setValue(fields.cert, certificate.certFile, { shouldDirty: true, shouldValidate: true });
    }
    if (currentKey !== certificate.keyFile) {
      setValue(fields.key, certificate.keyFile, { shouldDirty: true, shouldValidate: true });
    }
    if (fields.autoTLS) setValue(fields.autoTLS, false, { shouldDirty: true });
  }, [certificate, fields?.cert, fields?.key, fields?.autoTLS, getValues, local, nodeId, setValue, sites]);

  if (!local || !fields || !validSni || disabled) return null;

  const issue = async () => {
    if (issuing || !email.trim() || !consent) return;
    setIssuing(true);
    setError('');
    try {
      const response = await HttpUtil.post('/panel/api/manager/domains/local-certificate', {
        domain: normalized, email: email.trim(), acceptTerms: true,
      }, { headers: { 'Content-Type': 'application/json' }, silent: true });
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
            Нет действующего сертификата для {normalized} на основной ноде.
          </Typography.Text>
          <Button size="small" onClick={() => setOpen(true)}>Выпустить сертификат (DNS-01)</Button>
          {pending && <Button size="small" onClick={() => void refetch()}>Проверить выпуск ({pending})</Button>}
        </Space>
      )}
      <Modal
        open={open}
        title="TLS-сертификат на основной ноде"
        onCancel={() => setOpen(false)}
        onOk={() => void issue()}
        okText="Выпустить через DNS-01"
        confirmLoading={issuing}
        okButtonProps={{ disabled: !email.trim() || !consent }}
      >
        <Space orientation="vertical" style={{ width: '100%' }}>
          <Alert
            type="info"
            showIcon
            description="Используется DNS-01 провайдер основной ноды. Файл сертификата и приватный ключ будут храниться локально, без копирования с другого сервера. DNS-01 нужно настроить в разделе «Домены и TLS»."
          />
          <Typography.Text>{normalized}</Typography.Text>
          <Input type="email" maxLength={254} value={email} onChange={(e) => setEmail(e.target.value)} placeholder="Email для Let's Encrypt" />
          <Checkbox checked={consent} onChange={(e) => setConsent(e.target.checked)}>
            Я владею доменом и принимаю условия Let’s Encrypt.
          </Checkbox>
          {error && <Alert showIcon type="error" title={error} />}
        </Space>
      </Modal>
    </div>
  );
}
