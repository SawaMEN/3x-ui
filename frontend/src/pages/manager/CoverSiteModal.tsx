import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Checkbox, Input, Modal, Select, Space, Typography } from 'antd';
import type { NodeRecord } from '@/schemas/node';
import { HttpUtil } from '@/utils';

interface Props {
  open: boolean;
  nodes: NodeRecord[];
  onClose: () => void;
  onSuccess: (domain: string, state: string) => void;
}

export default function CoverSiteModal({ open, nodes, onClose, onSuccess }: Props) {
  const { i18n } = useTranslation();
  const ru = i18n.language.startsWith('ru');
  const t = (russian: string, english: string) => (ru ? russian : english);
  const [nodeId, setNodeId] = useState(0);
  const [domain, setDomain] = useState('');
  const [email, setEmail] = useState('');
  const [acceptTerms, setAcceptTerms] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const online = nodes.filter(
    (node) => node.id > 0 && node.enable && node.status === 'online' && !node.transitive,
  );
  const submit = async () => {
    if (saving || !domain.trim() || !email.trim() || !acceptTerms) return;
    setSaving(true);
    setError('');
    try {
      const result = await HttpUtil.post(
        '/panel/api/manager/domains/cover-site',
        { nodeId, domain: domain.trim(), email: email.trim(), acceptTerms },
        { headers: { 'Content-Type': 'application/json' }, silent: true },
      );
      if (!result?.success) {
        setError(result?.msg || t('Не удалось создать сайт', 'Failed to provision the website'));
        return;
      }
      const value = result.obj as { domain?: string; state?: string } | undefined;
      onSuccess(value?.domain || domain.trim(), value?.state || 'pending');
      setDomain('');
      setAcceptTerms(false);
      onClose();
    } catch (err) {
      setError(String(err));
    } finally {
      setSaving(false);
    }
  };
  return (
    <Modal
      open={open}
      title={t(
        'Собственный сайт-заглушка с автоматическим TLS',
        'Owned HTTPS cover site with automatic TLS',
      )}
      okText={t('Настроить сайт и сертификат', 'Provision site and certificate')}
      onOk={() => void submit()}
      onCancel={onClose}
      confirmLoading={saving}
      okButtonProps={{ disabled: !domain.trim() || !email.trim() || !acceptTerms }}
      destroyOnHidden
    >
      <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
        <Alert
          type="info"
          showIcon
          title={t(
            'Это собственный HTTPS-сайт, не внешний REALITY Target.',
            'This is an owned HTTPS website, not an external REALITY Target.',
          )}
          description={t(
            'Будет использован существующий Domain Front и CertMagic, включая автоматическое продление сертификата. Нужны контроль DNS домена и доступность ACME-проверок на выбранной ноде. Конфликт порта 443 или существующего сайта остановит настройку без замены его конфигурации.',
            'Uses existing Domain Front and CertMagic including automated renewal. You must control the DNS hostname and allow ACME challenges to reach the selected node. Conflicting port 443 or an existing site prevents provisioning without overwriting it.',
          )}
        />
        <div>
          <Typography.Text>{t('Нода', 'Node')}</Typography.Text>
          <Select
            value={nodeId}
            onChange={setNodeId}
            style={{ width: '100%' }}
            options={[
              { value: 0, label: t('Главный сервер', 'Main server') },
              ...online.map((node) => ({ value: node.id, label: node.name || `#${node.id}` })),
            ]}
          />
        </div>
        <div>
          <Typography.Text>{t('Мой домен', 'My DNS hostname')}</Typography.Text>
          <Input
            value={domain}
            onChange={(ev) => setDomain(ev.target.value)}
            placeholder="decoy.example.org"
            maxLength={253}
          />
        </div>
        <div>
          <Typography.Text>{t('Контактный email для ACME', 'ACME contact email')}</Typography.Text>
          <Input
            value={email}
            onChange={(ev) => setEmail(ev.target.value)}
            placeholder="admin@example.org"
            type="email"
            maxLength={254}
          />
        </div>
        <Checkbox checked={acceptTerms} onChange={(ev) => setAcceptTerms(ev.target.checked)}>
          {t(
            'Я контролирую этот домен и принимаю условия Let’s Encrypt.',
            'I control this hostname and accept the Let’s Encrypt terms.',
          )}
        </Checkbox>
        {error && <Alert type="error" showIcon title={error} />}
      </Space>
    </Modal>
  );
}
