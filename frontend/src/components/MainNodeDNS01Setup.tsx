import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Input, Modal, Select, Space, Typography } from 'antd';
import { useDomainFrontStatus } from '@/api/queries/useDomainFrontStatus';
import { keys } from '@/api/queryKeys';
import { HttpUtil } from '@/utils';

// Shared by certificate issuance entry points so a missing DNS configuration
// always offers an actionable next step without leaving the current form.
export default function MainNodeDNS01Setup() {
  const { i18n } = useTranslation();
  const t = (ru: string, en: string) => (i18n.language.startsWith('ru') ? ru : en);
  const status = useDomainFrontStatus();
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
        throw new Error(
          response?.msg || t('Не удалось проверить DNS-токен', 'Could not check DNS token'),
        );
      return response.obj as { configured: boolean };
    },
  });
  const edit = () => {
    const config = status.data?.config;
    setProvider(config?.dns01Provider || 'cloudflare');
    setZone(config?.dns01Zone || '');
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
      // Read the current configuration again: another issuance flow may have
      // added domains while this dialog was open. Preserve all other settings.
      const current = await status.refetch();
      if (current.isError || !current.data)
        throw new Error(
          t('Не удалось загрузить настройки доменов', 'Could not load domain settings'),
        );
      if (token) {
        const saved = await HttpUtil.post(
          '/panel/api/domains/dns/credential',
          { provider, token },
          { silent: true },
        );
        if (!saved?.success)
          throw new Error(
            saved?.msg || t('Не удалось сохранить DNS-токен', 'Could not save DNS token'),
          );
        setToken('');
      }
      const checked = await credential.refetch();
      if (checked.isError || !checked.data?.configured)
        throw new Error(
          t(
            'Укажите действующий API-токен DNS-провайдера',
            'Provide a valid DNS provider API token',
          ),
        );
      const response = await HttpUtil.post(
        '/panel/api/domains/apply',
        {
          ...current.data.config,
          dns01Provider: provider,
          dns01Zone: zone.trim().toLowerCase().replace(/\.$/, ''),
          dns01ZoneId: provider === 'cloudflare' ? zoneId.trim() : '',
        },
        { silent: true },
      );
      if (!response?.success)
        throw new Error(
          response?.msg || t('Не удалось настроить DNS-01', 'Could not configure DNS-01'),
        );
      await queryClient.invalidateQueries({ queryKey: keys.settings.domains() });
      setOpen(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };
  return (
    <>
      <Space wrap>
        <Typography.Text type="secondary">
          {status.data?.config.dns01Provider
            ? t('DNS-01 основной ноды:', 'Main-node DNS-01:') +
              ' ' +
              status.data.config.dns01Provider +
              ' · ' +
              status.data.config.dns01Zone
            : t(
                'Для выпуска нужен DNS-01 на основной ноде.',
                'Issuance requires DNS-01 on the main node.',
              )}
        </Typography.Text>
        <Button size="small" loading={status.isPending} disabled={!status.data} onClick={edit}>
          {t('Настроить DNS-01', 'Configure DNS-01')}
        </Button>
        {status.isError && (
          <Button size="small" onClick={() => void status.refetch()}>
            {t('Повторить загрузку', 'Retry loading')}
          </Button>
        )}
      </Space>
      <Modal
        open={open}
        title={t('DNS-01 основной ноды', 'Main-node DNS-01')}
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
              'Выберите провайдера, который обслуживает DNS вашего домена. Для Cloudflare нужны Zone ID и API-токен с правом изменения DNS этой зоны; для DigitalOcean — токен с доступом к DNS. После сохранения вернитесь к выпуску сертификата.',
              'Choose the provider hosting your domain DNS. Cloudflare requires a Zone ID and an API token allowed to edit DNS in that zone; DigitalOcean requires a token with DNS access. After saving, continue certificate issuance.',
            )}
          />
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
          <Input
            aria-label={t('DNS-зона', 'DNS zone')}
            placeholder="example.org"
            value={zone}
            disabled={saving}
            onChange={(e) => setZone(e.target.value)}
          />
          {provider === 'cloudflare' && (
            <Input
              aria-label="Cloudflare Zone ID"
              placeholder="Cloudflare Zone ID"
              value={zoneId}
              disabled={saving}
              onChange={(e) => setZoneId(e.target.value)}
            />
          )}
          <Input.Password
            aria-label={t('API-токен DNS', 'DNS API token')}
            placeholder={
              credential.data?.configured
                ? t(
                    'Токен сохранён; оставьте пустым для повторного использования',
                    'Token saved; leave empty to reuse',
                  )
                : t('API-токен DNS', 'DNS API token')
            }
            value={token}
            autoComplete="new-password"
            disabled={saving}
            onChange={(e) => setToken(e.target.value)}
          />
          {credential.isError && <Alert type="error" title={credential.error.message} />}
          {error && <Alert type="error" showIcon title={error} />}
        </Space>
      </Modal>
    </>
  );
}
