import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Checkbox, Input, Modal, Select, Space, Typography } from 'antd';
import type { NodeRecord } from '@/schemas/node';
import { HttpUtil } from '@/utils';
import { TelemtWebCatalogs } from '@/schemas/telemt-web-catalog';

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
  const [siteCatalog, setSiteCatalog] = useState<'builtin' | 'mtproxyl'>('builtin');
  const [siteTemplate, setSiteTemplate] = useState('notes');
  const [issueOnMain, setIssueOnMain] = useState(false);
  const [acceptTerms, setAcceptTerms] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [mainCertificate, setMainCertificate] = useState<{
    domain: string;
    state: string;
    certFile: string;
    keyFile: string;
  } | null>(null);
  const [checkingCertificate, setCheckingCertificate] = useState(false);
  const checkMainCertificate = async () => {
    if (!mainCertificate || checkingCertificate) return;
    setCheckingCertificate(true);
    try {
      const response = await HttpUtil.get('/panel/api/domains/sni', undefined, { silent: true });
      if (!response?.success || !Array.isArray(response.obj)) {
        throw new Error(response?.msg || t('Не удалось обновить сертификат', 'Could not refresh certificate'));
      }
      const matching = (response.obj as Array<{
        name: string;
        nodeId?: number;
        certFile?: string;
        keyFile?: string;
      }>).find((entry) =>
        entry.name === mainCertificate.domain && !entry.nodeId &&
        entry.certFile && entry.keyFile,
      );
      if (matching) {
        setMainCertificate((current) => current && ({
          ...current,
          state: 'valid',
          certFile: matching.certFile || '',
          keyFile: matching.keyFile || '',
        }));
      } else {
        setError(t('Сертификат ещё не готов. Проверьте DNS-01 и повторите позже.', 'Certificate is not ready yet. Check DNS-01 and retry later.'));
      }
    } catch (err) {
      setError(String(err));
    } finally {
      setCheckingCertificate(false);
    }
  };
  const online = nodes.filter(
    (node) => node.id > 0 && node.enable && node.status === 'online' && !node.transitive,
  );
  const submit = async () => {
    if (saving || mainCertificate || !domain.trim() || !email.trim() || !acceptTerms) return;
    setSaving(true);
    setError('');
    try {
      const result = await HttpUtil.post(
        '/panel/api/manager/domains/cover-site',
        { nodeId, domain: domain.trim(), email: email.trim(), acceptTerms, siteCatalog, siteTemplate, issueOnMain: nodeId > 0 && issueOnMain },
        { headers: { 'Content-Type': 'application/json' }, silent: true },
      );
      if (!result?.success) {
        setError(result?.msg || t('Не удалось создать сайт', 'Failed to provision the website'));
        return;
      }
      const value = result.obj as {
        domain?: string;
        state?: string;
        mainError?: string;
        mainState?: string;
        mainCertFile?: string;
        mainKeyFile?: string;
      } | undefined;
      onSuccess(value?.domain || domain.trim(), value?.state || 'pending');
      if (nodeId > 0 && issueOnMain) {
        setMainCertificate({
          domain: value?.domain || domain.trim().toLowerCase().replace(/\.$/, ''),
          state: value?.mainState || 'pending',
          certFile: value?.mainCertFile || '',
          keyFile: value?.mainKeyFile || '',
        });
      }
      if (value?.mainError) {
        setError(t('Удалённый сайт создан, но выпуск на основной ноде не удался: ', 'Remote site succeeded, but main-node certificate failed: ') + value.mainError);
        return;
      }
      if (nodeId > 0 && issueOnMain) return;
      setDomain('');
      setSiteCatalog('builtin');
      setSiteTemplate('notes');
      setAcceptTerms(false);
      setIssueOnMain(false);
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
      okText={mainCertificate ? t('Закрыть', 'Close') : t('Настроить сайт и сертификат', 'Provision site and certificate')}
      onOk={() => mainCertificate ? onClose() : void submit()}
      onCancel={onClose}
      confirmLoading={saving}
      okButtonProps={{ disabled: !mainCertificate && (!domain.trim() || !email.trim() || !acceptTerms) }}
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
          <Typography.Text>{t('Каталог сайта-заглушки', 'Cover site catalog')}</Typography.Text>
          <Select
            value={siteCatalog}
            onChange={(value) => {
              setSiteCatalog(value);
              setSiteTemplate(value === 'builtin' ? 'notes' : 'stub');
            }}
            style={{ width: '100%' }}
            options={Object.entries(TelemtWebCatalogs).map(([value, entry]) => ({
              value,
              label: ru ? entry.ru : entry.en,
            }))}
          />
        </div>
        <div>
          <Typography.Text>{t('Какой сайт создать', 'Website template')}</Typography.Text>
          <Select
            value={siteTemplate}
            onChange={setSiteTemplate}
            style={{ width: '100%' }}
            options={TelemtWebCatalogs[siteCatalog].templates.map((entry) => ({
              value: entry.id,
              label: ru ? entry.ru : entry.en,
              title: ru ? entry.descriptionRu : entry.descriptionEn,
            }))}
          />
          <Typography.Text type="secondary" style={{ display: 'block', marginTop: 4 }}>
            {t('Для изменения существующей заглушки укажите ту же ноду и домен и выберите другой сайт.', 'To edit an existing cover site, select the same node and domain with a different template.')}
          </Typography.Text>
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
        {nodeId > 0 && (
          <Checkbox checked={issueOnMain} onChange={(ev) => setIssueOnMain(ev.target.checked)}>
            {t('Дополнительно выпустить сертификат на основной ноде (DNS-01)', 'Also issue a certificate on the main node (DNS-01)')}
          </Checkbox>
        )}
        {nodeId > 0 && issueOnMain && (
          <Alert
            type="info"
            showIcon
            title={t(
              'Настройте DNS-01 (Cloudflare или DigitalOcean) на основной ноде в «Домены и TLS».',
              'Configure DNS-01 (Cloudflare or DigitalOcean) on the main node under Domains & TLS.',
            )}
            description={t(
              'Сертификат и приватный ключ создаются на основном сервере без копирования с удалённого и без занятия портов 80/443. После выпуска локальные пути доступны в списке сертификатов и TLS/SNI.',
              'The certificate and private key are issued locally without copying from the remote server or binding ports 80/443. After issuance, local paths are available in the certificate and TLS/SNI lists.',
            )}
          />
        )}
        <Checkbox checked={acceptTerms} onChange={(ev) => setAcceptTerms(ev.target.checked)}>
          {t(
            'Я контролирую этот домен и принимаю условия Let’s Encrypt.',
            'I control this hostname and accept the Let’s Encrypt terms.',
          )}
        </Checkbox>
        {mainCertificate && (
          <Alert
            type="success"
            showIcon
            title={t('Сайт-заглушка создан на удалённой ноде', 'Cover site created on remote node')}
            description={
              <Space orientation="vertical" size="small">
                <Typography.Text>
                  {t('Сертификат на основной ноде:', 'Main-node certificate:')} {mainCertificate.state}
                </Typography.Text>
                {mainCertificate.certFile && mainCertificate.keyFile ? (
                  <>
                    <Typography.Text copyable={{ text: mainCertificate.certFile }}>
                      certFile: {mainCertificate.certFile}
                    </Typography.Text>
                    <Typography.Text copyable={{ text: mainCertificate.keyFile }}>
                      keyFile: {mainCertificate.keyFile}
                    </Typography.Text>
                  </>
                ) : (
                  <Button loading={checkingCertificate} onClick={() => void checkMainCertificate()}>
                    {t('Проверить выпуск сертификата', 'Check certificate issuance')}
                  </Button>
                )}
              </Space>
            }
          />
        )}
        {error && <Alert type="error" showIcon title={error} />}
      </Space>
    </Modal>
  );
}
