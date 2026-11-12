import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Input,
  Modal,
  Segmented,
  Select,
  Space,
  Typography,
} from 'antd';
import type { NodeRecord } from '@/schemas/node';
import { HttpUtil } from '@/utils';
import MainNodeDNS01Setup, { useMainNodeDNS01Readiness } from '@/components/MainNodeDNS01Setup';
import { TelemtWebCatalogs } from '@/schemas/telemt-web-catalog';
import CoverSiteHTMLEditor, {
  coverSiteHTMLValidation,
  starterCoverSiteHTML,
} from '@/components/CoverSiteHTMLEditor';

type EditableCoverSite = {
  nodeId: number;
  domain: string;
  siteCatalog: string;
  siteTemplate: string;
};

interface Props {
  open: boolean;
  nodes: NodeRecord[];
  onClose: () => void;
  onSuccess: (domain: string, state: string) => void;
}

export default function CoverSiteModal({ open, nodes, onClose, onSuccess }: Props) {
  const { i18n } = useTranslation();
  const queryClient = useQueryClient();
  const ru = i18n.language.startsWith('ru');
  const t = (russian: string, english: string) => (ru ? russian : english);
  const [nodeId, setNodeId] = useState(0);
  const { data: existingSites = [] } = useQuery({
    queryKey: ['manager', 'cover-sites'],
    enabled: open,
    staleTime: 30_000,
    retry: false,
    queryFn: async (): Promise<EditableCoverSite[]> => {
      const result = await HttpUtil.get('/panel/api/domains/cover-sites', undefined, {
        silent: true,
      });
      if (!result?.success || !Array.isArray(result.obj)) return [];
      return result.obj as EditableCoverSite[];
    },
  });
  const [domain, setDomain] = useState('');
  const [email, setEmail] = useState('');
  const [siteCatalog, setSiteCatalog] = useState<'builtin' | 'mtproxyl' | 'custom'>('builtin');
  const [siteTemplate, setSiteTemplate] = useState('notes');
  const [editedHTML, setEditedHTML] = useState<string | null>(null);
  const [issueOnMain, setIssueOnMain] = useState(false);
  const dns01 = useMainNodeDNS01Readiness(domain);
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
        throw new Error(
          response?.msg || t('Не удалось обновить сертификат', 'Could not refresh certificate'),
        );
      }
      const matching = (
        response.obj as Array<{
          name: string;
          nodeId?: number;
          certFile?: string;
          keyFile?: string;
        }>
      ).find(
        (entry) =>
          entry.name === mainCertificate.domain && !entry.nodeId && entry.certFile && entry.keyFile,
      );
      if (matching) {
        setMainCertificate(
          (current) =>
            current && {
              ...current,
              state: 'valid',
              certFile: matching.certFile || '',
              keyFile: matching.keyFile || '',
            },
        );
      } else {
        setError(
          t(
            'Сертификат ещё не готов. Проверьте DNS-01 и повторите позже.',
            'Certificate is not ready yet. Check DNS-01 and retry later.',
          ),
        );
      }
    } catch (err) {
      setError(String(err));
    } finally {
      setCheckingCertificate(false);
    }
  };
  const selectedExisting = existingSites.find(
    (site) => site.nodeId === nodeId && site.domain === domain.trim().toLowerCase(),
  );
  const nodeSites = existingSites.filter((site) => site.nodeId === nodeId);
  const isCustom = siteCatalog === 'custom';
  const {
    data: savedContent,
    isFetching: loadingContent,
    error: contentError,
  } = useQuery({
    queryKey: ['manager', 'cover-site-content', nodeId, selectedExisting?.domain],
    enabled: open && selectedExisting?.siteCatalog === 'custom',
    staleTime: Infinity,
    retry: false,
    queryFn: async (): Promise<{ domain: string; nodeId: number; html: string }> => {
      const selected = selectedExisting;
      if (!selected) throw new Error('Cover site not selected');
      const response = await HttpUtil.get<{ html: string }>(
        '/panel/api/domains/cover-sites/content',
        { nodeId: selected.nodeId, domain: selected.domain },
        { silent: true },
      );
      if (!response?.success || typeof response.obj?.html !== 'string') {
        throw new Error(response?.msg || t('Не удалось загрузить HTML', 'Unable to load HTML'));
      }
      return { nodeId: selected.nodeId, domain: selected.domain, html: response.obj.html };
    },
  });
  const siteHTML =
    editedHTML ??
    (selectedExisting?.siteCatalog === 'custom'
      ? (savedContent?.html ?? '')
      : starterCoverSiteHTML);
  const htmlError = isCustom ? coverSiteHTMLValidation(siteHTML) : null;
  const setSiteHTML = setEditedHTML;

  const chooseExisting = (value?: string) => {
    setEditedHTML(null);
    const selected = nodeSites.find((site) => site.domain === value);
    if (!selected) {
      setDomain('');
      return;
    }
    if (selected.siteCatalog === 'custom' && selected.siteTemplate === 'html') {
      setSiteCatalog('custom');
      setSiteTemplate('html');
    } else {
      const catalog = selected.siteCatalog === 'mtproxyl' ? 'mtproxyl' : 'builtin';
      const template = TelemtWebCatalogs[catalog].templates.some(
        (item) => item.id === selected.siteTemplate,
      )
        ? selected.siteTemplate
        : catalog === 'builtin'
          ? 'notes'
          : 'stub';
      setSiteCatalog(catalog);
      setSiteTemplate(template);
    }
    setDomain(selected.domain);
  };
  const online = nodes.filter(
    (node) => node.id > 0 && node.enable && node.status === 'online' && !node.transitive,
  );
  const submit = async (dnsJustConfigured = false) => {
    if (saving || mainCertificate || !domain.trim() || htmlError) return;
    if (isCustom && selectedExisting?.siteCatalog === 'custom' && (loadingContent || contentError))
      return;
    if (nodeId > 0 && issueOnMain && !dns01.ready && !dnsJustConfigured) return;
    if (!selectedExisting && (!email.trim() || !acceptTerms)) return;
    setSaving(true);
    setError('');
    try {
      if (selectedExisting) {
        const response = await HttpUtil.post(
          '/panel/api/domains/cover-sites/change-template',
          {
            nodeId,
            domain: selectedExisting.domain,
            siteCatalog,
            siteTemplate,
            siteHtml: isCustom ? siteHTML : '',
          },
          { headers: { 'Content-Type': 'application/json' }, silent: true },
        );
        if (!response?.success) {
          throw new Error(
            response?.msg || t('Не удалось сменить сайт', 'Could not change cover site'),
          );
        }
        void queryClient.invalidateQueries({ queryKey: ['manager', 'cover-sites'] });
        void queryClient.invalidateQueries({
          queryKey: ['manager', 'cover-site-content', nodeId, selectedExisting.domain],
        });
        void queryClient.invalidateQueries({ queryKey: ['settings', 'managed-sni'] });
        onSuccess(selectedExisting.domain, 'valid');
        setDomain('');
        onClose();
        return;
      }
      const result = await HttpUtil.post(
        '/panel/api/manager/domains/cover-site',
        {
          nodeId,
          domain: domain.trim(),
          email: email.trim(),
          acceptTerms,
          siteCatalog,
          siteTemplate,
          siteHtml: isCustom ? siteHTML : '',
          issueOnMain: nodeId > 0 && issueOnMain,
        },
        { headers: { 'Content-Type': 'application/json' }, silent: true },
      );
      if (!result?.success) {
        setError(result?.msg || t('Не удалось создать сайт', 'Failed to provision the website'));
        return;
      }
      const value = result.obj as
        | {
            domain?: string;
            state?: string;
            mainError?: string;
            mainState?: string;
            mainCertFile?: string;
            mainKeyFile?: string;
          }
        | undefined;
      onSuccess(value?.domain || domain.trim(), value?.state || 'pending');
      void queryClient.invalidateQueries({ queryKey: ['manager', 'cover-sites'] });
      void queryClient.invalidateQueries({ queryKey: ['settings', 'managed-sni'] });
      if (nodeId > 0 && issueOnMain) {
        setMainCertificate({
          domain: value?.domain || domain.trim().toLowerCase().replace(/\.$/, ''),
          state: value?.mainState || 'pending',
          certFile: value?.mainCertFile || '',
          keyFile: value?.mainKeyFile || '',
        });
      }
      if (value?.mainError) {
        setError(
          t(
            'Удалённый сайт создан, но выпуск на основной ноде не удался: ',
            'Remote site succeeded, but main-node certificate failed: ',
          ) + value.mainError,
        );
        return;
      }
      if (nodeId > 0 && issueOnMain) return;
      setDomain('');
      setSiteCatalog('builtin');
      setSiteTemplate('notes');
      setEditedHTML(null);
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
      width={760}
      title={t(
        'Собственный сайт-заглушка с автоматическим TLS',
        'Owned HTTPS cover site with automatic TLS',
      )}
      okText={
        mainCertificate
          ? t('Закрыть', 'Close')
          : selectedExisting
            ? t('Сменить сайт-заглушку', 'Change cover site')
            : t('Настроить сайт и сертификат', 'Provision site and certificate')
      }
      onOk={() => (mainCertificate ? onClose() : void submit())}
      onCancel={onClose}
      confirmLoading={saving}
      okButtonProps={{
        disabled:
          !mainCertificate &&
          (!domain.trim() ||
            Boolean(htmlError) ||
            (isCustom &&
              selectedExisting?.siteCatalog === 'custom' &&
              (loadingContent || Boolean(contentError))) ||
            (!selectedExisting &&
              (!email.trim() || !acceptTerms || (nodeId > 0 && issueOnMain && !dns01.ready)))),
      }}
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
            onChange={(nextNode: number) => {
              setNodeId(nextNode);
              setDomain('');
              setSiteCatalog('builtin');
              setSiteTemplate('notes');
              setEditedHTML(null);
              setIssueOnMain(false);
            }}
            style={{ width: '100%' }}
            options={[
              { value: 0, label: t('Главный сервер', 'Main server') },
              ...online.map((node) => ({ value: node.id, label: node.name || `#${node.id}` })),
            ]}
          />
        </div>
        <div>
          <Typography.Text>
            {t('Изменить существующий SNI-сайт', 'Change an existing SNI website')}
          </Typography.Text>
          <Select
            allowClear
            showSearch
            style={{ width: '100%' }}
            placeholder={t('Выберите уже созданную заглушку', 'Select an existing cover site')}
            value={selectedExisting?.domain}
            options={nodeSites.map((site) => ({ value: site.domain, label: site.domain }))}
            onChange={(value) => chooseExisting(value)}
          />
          {selectedExisting && (
            <Typography.Text type="secondary" style={{ display: 'block', marginTop: 4 }}>
              {t(
                'Можно заменить шаблон или изменить собственный HTML. Домен и сертификат сохранятся.',
                'You can change the template or edit your HTML. The domain and certificate stay unchanged.',
              )}
            </Typography.Text>
          )}
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
        <Card size="small" title={t('Страница сайта-заглушки', 'Cover website page')}>
          <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
            <Segmented
              block
              value={isCustom ? 'custom' : 'templates'}
              onChange={(value) => {
                if (value === 'custom') {
                  setSiteCatalog('custom');
                  setSiteTemplate('html');
                } else {
                  setSiteCatalog('builtin');
                  setSiteTemplate('notes');
                }
              }}
              options={[
                { value: 'templates', label: t('Готовая страница', 'Ready-made template') },
                { value: 'custom', label: t('Свой HTML', 'Custom HTML') },
              ]}
            />
            {isCustom ? (
              <>
                {selectedExisting?.siteCatalog === 'custom' && loadingContent && (
                  <Typography.Text type="secondary">
                    {t('Загружаем сохранённую страницу…', 'Loading saved website…')}
                  </Typography.Text>
                )}
                {selectedExisting?.siteCatalog === 'custom' && contentError && (
                  <Alert
                    type="error"
                    showIcon
                    title={t(
                      'Не удалось загрузить сохранённый HTML. Повторите открытие страницы.',
                      'Could not load saved HTML. Reopen the page and retry.',
                    )}
                  />
                )}
                <CoverSiteHTMLEditor
                  value={siteHTML}
                  onChange={setSiteHTML}
                  disabled={selectedExisting?.siteCatalog === 'custom' && loadingContent}
                />
              </>
            ) : (
              <>
                <div>
                  <Typography.Text>{t('Коллекция', 'Collection')}</Typography.Text>
                  <Select
                    value={siteCatalog}
                    onChange={(value: 'builtin' | 'mtproxyl') => {
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
                  <Typography.Text>{t('Дизайн страницы', 'Page design')}</Typography.Text>
                  <Select
                    showSearch={{ optionFilterProp: 'label' }}
                    value={siteTemplate}
                    onChange={setSiteTemplate}
                    style={{ width: '100%' }}
                    options={TelemtWebCatalogs[
                      siteCatalog === 'mtproxyl' ? 'mtproxyl' : 'builtin'
                    ].templates.map((entry) => ({
                      value: entry.id,
                      label: ru ? entry.ru : entry.en,
                    }))}
                  />
                  <Typography.Text type="secondary" style={{ display: 'block', marginTop: 6 }}>
                    {(() => {
                      const selected = TelemtWebCatalogs[
                        siteCatalog === 'mtproxyl' ? 'mtproxyl' : 'builtin'
                      ].templates.find((entry) => entry.id === siteTemplate);
                      return selected ? (ru ? selected.descriptionRu : selected.descriptionEn) : '';
                    })()}
                  </Typography.Text>
                </div>
              </>
            )}
            <Typography.Text type="secondary">
              {t(
                'После сохранения выбранная страница автоматически размещается на ноде. Смена оформления не меняет TLS и маршруты.',
                'After saving, the selected page is published on the node automatically. TLS and routes remain unchanged.',
              )}
            </Typography.Text>
          </Space>
        </Card>
        {!selectedExisting && (
          <div>
            <Typography.Text>
              {t('Контактный email для ACME', 'ACME contact email')}
            </Typography.Text>
            <Input
              value={email}
              onChange={(ev) => setEmail(ev.target.value)}
              placeholder="admin@example.org"
              type="email"
              maxLength={254}
            />
          </div>
        )}
        {nodeId > 0 && !selectedExisting && (
          <Checkbox checked={issueOnMain} onChange={(ev) => setIssueOnMain(ev.target.checked)}>
            {t(
              'Дополнительно выпустить сертификат на основной ноде (DNS-01)',
              'Also issue a certificate on the main node (DNS-01)',
            )}
          </Checkbox>
        )}
        {nodeId > 0 && !selectedExisting && issueOnMain && (
          <Alert
            type="info"
            showIcon
            title={t(
              'Сертификат на основной ноде требует доступа к DNS выбранного домена. Настройка ниже проверяется до создания сайта.',
              'The main-node certificate requires DNS access for this hostname. Setup is checked before provisioning.',
            )}
            description={t(
              'Сертификат и приватный ключ создаются на основном сервере без копирования с удалённого и без занятия портов 80/443. После выпуска локальные пути доступны в списке сертификатов и TLS/SNI.',
              'The certificate and private key are issued locally without copying from the remote server or binding ports 80/443. After issuance, local paths are available in the certificate and TLS/SNI lists.',
            )}
          />
        )}
        {nodeId > 0 && !selectedExisting && issueOnMain && (
          <MainNodeDNS01Setup
            domain={domain}
            onConfigured={() => {
              if (domain.trim() && email.trim() && acceptTerms) void submit(true);
            }}
          />
        )}
        {!selectedExisting && (
          <Checkbox checked={acceptTerms} onChange={(ev) => setAcceptTerms(ev.target.checked)}>
            {t(
              'Я контролирую этот домен и принимаю условия Let’s Encrypt.',
              'I control this hostname and accept the Let’s Encrypt terms.',
            )}
          </Checkbox>
        )}
        {mainCertificate && (
          <Alert
            type="success"
            showIcon
            title={t('Сайт-заглушка создан на удалённой ноде', 'Cover site created on remote node')}
            description={
              <Space orientation="vertical" size="small">
                <Typography.Text>
                  {t('Сертификат на основной ноде:', 'Main-node certificate:')}{' '}
                  {mainCertificate.state}
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
