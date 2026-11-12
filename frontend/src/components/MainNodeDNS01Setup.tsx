import { useState } from 'react';
import { QuestionCircleOutlined } from '@ant-design/icons';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Input, Modal, Select, Space, Tooltip, Typography } from 'antd';
import { useDomainFrontStatus } from '@/api/queries/useDomainFrontStatus';
import { keys } from '@/api/queryKeys';
import { HttpUtil } from '@/utils';

type Props = {
  domain?: string;
  onConfigured?: () => void;
};

const normalizeName = (value: string) => value.trim().toLowerCase().replace(/\.$/, '');

// Only a suggestion: public suffixes vary, so the DNS zone remains editable.
function suggestZone(hostname: string): string {
  const labels = normalizeName(hostname).split('.');
  if (labels.length < 2 || labels.some((part) => !/^[a-z0-9-]+$/.test(part))) return '';
  const suffix = labels.slice(-2).join('.');
  const nestedSuffix = /^(?:co|com|net|org|gov|edu|ac)\.[a-z]{2}$/.test(suffix);
  return labels.slice(-(nestedSuffix && labels.length >= 3 ? 3 : 2)).join('.');
}

// Shared by certificate entry points. Do not try ACME until both the
// configuration and the provider credential are present and cover the SNI.
export function useMainNodeDNS01Readiness(domain = '') {
  const status = useDomainFrontStatus();
  const config = status.data?.config;
  const provider = config?.dns01Provider || '';
  const credential = useQuery({
    queryKey: ['settings', 'domains', 'dns-credential', provider],
    enabled: !!provider,
    retry: false,
    staleTime: 30_000,
    queryFn: async () => {
      const response = await HttpUtil.get(
        `/panel/api/domains/dns/credential?provider=${provider}`,
        undefined,
        { silent: true },
      );
      if (!response?.success) throw new Error(response?.msg || 'Could not check DNS credential');
      return response.obj as { configured: boolean; source?: string };
    },
  });
  const hostname = normalizeName(domain);
  const zone = config?.dns01Zone || '';
  const coversDomain = !!zone && (!hostname || hostname === zone || hostname.endsWith('.' + zone));
  const ready =
    !!provider &&
    coversDomain &&
    (provider !== 'cloudflare' || /^[a-f0-9]{32}$/i.test(config?.dns01ZoneId || '')) &&
    credential.data?.configured === true;
  return {
    status,
    credential,
    config,
    ready,
    coversDomain,
    checking: status.isPending || (!!provider && credential.isPending),
  };
}

function HelpLabel({ label, help }: { label: string; help: string }) {
  return (
    <Space size={5} style={{ marginBottom: 4 }}>
      <Typography.Text>{label}</Typography.Text>
      <Tooltip title={help}>
        <QuestionCircleOutlined aria-label={label + ' help'} style={{ cursor: 'help' }} />
      </Tooltip>
    </Space>
  );
}

// Tokens are stored on the main server, not in the node configuration.
// After setup the caller may continue the pending certificate operation.
export default function MainNodeDNS01Setup({ domain = '', onConfigured }: Props) {
  const { i18n } = useTranslation();
  const t = (ru: string, en: string) => (i18n.language.startsWith('ru') ? ru : en);
  const readiness = useMainNodeDNS01Readiness(domain);
  const { status, config } = readiness;
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [provider, setProvider] = useState('cloudflare');
  const [zone, setZone] = useState('');
  const [zoneId, setZoneId] = useState('');
  const [token, setToken] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const credential = useQuery({
    queryKey: ['settings', 'domains', 'dns-credential', provider],
    enabled: open,
    retry: false,
    queryFn: async () => {
      const response = await HttpUtil.get(
        `/panel/api/domains/dns/credential?provider=${provider}`,
        undefined,
        { silent: true },
      );
      if (!response?.success)
        throw new Error(response?.msg || t('Не удалось проверить DNS-токен', 'Could not check DNS token'));
      return response.obj as { configured: boolean; source?: string };
    },
  });
  const edit = () => {
    const target = normalizeName(domain);
    const currentZone = config?.dns01Zone || '';
    const existingCovers = !!currentZone && (!target || target === currentZone || target.endsWith('.' + currentZone));
    setProvider(config?.dns01Provider || 'cloudflare');
    setZone(existingCovers ? currentZone : suggestZone(target) || currentZone);
    setZoneId(config?.dns01ZoneId || '');
    setToken('');
    setError('');
    setOpen(true);
  };
  const save = async () => {
    if (saving) return;
    setSaving(true);
    setError('');
    try {
      // Refetch before applying so concurrent domain changes are preserved.
      const current = await status.refetch();
      if (current.isError || !current.data)
        throw new Error(t('Не удалось загрузить настройки доменов', 'Could not load domain settings'));
      if (token) {
        const saved = await HttpUtil.post(
          '/panel/api/domains/dns/credential',
          { provider, token },
          { silent: true },
        );
        if (!saved?.success)
          throw new Error(saved?.msg || t('Не удалось сохранить DNS-токен', 'Could not save DNS token'));
        setToken('');
      }
      const checked = await credential.refetch();
      if (checked.isError || !checked.data?.configured)
        throw new Error(t('Укажите действующий API-токен DNS-провайдера', 'Provide a valid DNS provider API token'));
      const response = await HttpUtil.post(
        '/panel/api/domains/apply',
        {
          ...current.data.config,
          dns01Provider: provider,
          dns01Zone: normalizeName(zone),
          dns01ZoneId: provider === 'cloudflare' ? zoneId.trim() : '',
        },
        { silent: true },
      );
      if (!response?.success)
        throw new Error(response?.msg || t('Не удалось настроить DNS-01', 'Could not configure DNS-01'));
      await queryClient.invalidateQueries({ queryKey: keys.settings.domains() });
      await queryClient.invalidateQueries({ queryKey: ['settings', 'domains', 'dns-credential', provider] });
      setOpen(false);
      onConfigured?.();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };
  return (
    <>
      <Space wrap>
        <Typography.Text type={readiness.ready ? 'success' : 'secondary'}>
          {readiness.ready
            ? t('DNS-01 готов:', 'DNS-01 ready:') + ' ' + config?.dns01Provider + ' · ' + config?.dns01Zone
            : config?.dns01Provider && domain && !readiness.coversDomain
              ? t('DNS-зона не охватывает выбранный домен.', 'DNS zone does not cover this hostname.')
              : t('Для выпуска настройте DNS-01 основной ноды.', 'Configure main-node DNS-01 before issuing.')}
        </Typography.Text>
        <Tooltip title={t(
          'DNS-01 подтверждает владение доменом через временную TXT-запись. Нужен API-токен провайдера DNS, а не удалённой ноды. Порты 80/443 на основной ноде не требуются.',
          'DNS-01 verifies domain ownership using a temporary TXT record. You need a DNS provider API token, not credentials from the remote node. The main node does not need ports 80/443.',
        )}>
          <QuestionCircleOutlined aria-label="DNS-01 help" style={{ cursor: 'help' }} />
        </Tooltip>
        <Button size="small" loading={status.isPending} disabled={!status.data} onClick={edit}>
          {t('Настроить DNS-01', 'Configure DNS-01')}
        </Button>
        {status.isError && (
          <Button size="small" onClick={() => void status.refetch()}>
            {t('Повторить загрузку', 'Retry loading')}
          </Button>
        )}
        {readiness.credential.isError && (
          <Button size="small" onClick={() => void readiness.credential.refetch()}>
            {t('Повторить проверку токена', 'Retry token check')}
          </Button>
        )}
      </Space>
      <Modal
        open={open}
        title={t('Настройка DNS-01 на основном сервере', 'Configure DNS-01 on main server')}
        onCancel={() => {
          if (!saving) {
            setOpen(false);
            setToken('');
          }
        }}
        onOk={() => void save()}
        confirmLoading={saving}
        cancelButtonProps={{ disabled: saving }}
        okText={t('Сохранить DNS-01', 'Save DNS-01')}
        okButtonProps={{
          disabled:
            !zone.trim() ||
            (provider === 'cloudflare' && !/^[a-f0-9]{32}$/i.test(zoneId.trim())) ||
            credential.isFetching ||
            (!token && !credential.data?.configured),
        }}
      >
        <Space orientation="vertical" style={{ width: '100%' }}>
          <Alert
            type="info"
            showIcon
            description={t(
              'Зона подставлена по домену, проверьте её. Выберите DNS-провайдера, укажите необходимые данные и сохраните. Если email и согласие уже введены, выпуск сертификата продолжится автоматически.',
              'The DNS zone is suggested from the hostname; verify it. Choose the DNS provider, enter its credentials and save. If email and consent are already supplied, certificate issuance continues automatically.',
            )}
          />
          <div>
            <HelpLabel label={t('DNS-провайдер', 'DNS provider')} help={t(
              'Провайдер, на чьих DNS-серверах размещена зона домена. Выбор не связан с компанией, где арендована нода.',
              'The provider hosting this domain’s authoritative DNS zone, not necessarily the company hosting the node.',
            )} />
            <Select
              aria-label={t('DNS-провайдер', 'DNS provider')}
              value={provider}
              disabled={saving}
              onChange={(value) => {
                setProvider(value);
                setToken('');
              }}
              style={{ width: '100%' }}
              options={[
                { value: 'cloudflare', label: 'Cloudflare' },
                { value: 'digitalocean', label: 'DigitalOcean' },
              ]}
            />
          </div>
          <div>
            <HelpLabel label={t('DNS-зона', 'DNS zone')} help={t(
              'Основной DNS-домен, содержащий выбранное имя. Для vpn.example.org обычно example.org. Автоподстановка ориентировочная: проверьте делегирование и исправьте при необходимости.',
              'The authoritative DNS zone containing the hostname. For vpn.example.org it is usually example.org. The suggestion is approximate; verify DNS delegation and adjust it.',
            )} />
            <Input
              aria-label={t('DNS-зона', 'DNS zone')}
              placeholder="example.org"
              value={zone}
              disabled={saving}
              onChange={(e) => setZone(e.target.value)}
            />
          </div>
          {provider === 'cloudflare' && (
            <div>
              <HelpLabel label="Cloudflare Zone ID" help={t(
                '32-символьный идентификатор зоны в Cloudflare: выберите домен → Overview → Zone ID. Это не Account ID.',
                'The 32-character zone identifier in Cloudflare: select your domain → Overview → Zone ID. This is not the Account ID.',
              )} />
              <Input
                aria-label="Cloudflare Zone ID"
                placeholder="Cloudflare Zone ID"
                value={zoneId}
                disabled={saving}
                onChange={(e) => setZoneId(e.target.value)}
              />
            </div>
          )}
          <div>
            <HelpLabel label={t('API-токен DNS', 'DNS API token')} help={t(
              'Создайте токен с минимальными правами на редактирование DNS-записей выбранной зоны. Токен сохраняется в закрытом файле основной ноды и не отправляется на удалённые серверы.',
              'Create an API token scoped to DNS record editing for this zone. It is stored in a private file on the main node and never sent to remote nodes.',
            )} />
            <Input.Password
              aria-label={t('API-токен DNS', 'DNS API token')}
              placeholder={
                credential.data?.configured
                  ? t('Токен сохранён; оставьте пустым для повторного использования', 'Token saved; leave empty to reuse')
                  : t('API-токен DNS', 'DNS API token')
              }
              value={token}
              autoComplete="new-password"
              disabled={saving}
              onChange={(e) => setToken(e.target.value)}
            />
          </div>
          {credential.isError && <Alert type="error" title={credential.error.message} />}
          {error && <Alert type="error" showIcon title={error} />}
        </Space>
      </Modal>
    </>
  );
}
