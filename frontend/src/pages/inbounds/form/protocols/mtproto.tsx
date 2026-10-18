import { useTranslation } from 'react-i18next';
import { Alert, Collapse, Input, InputNumber, Select, Switch } from 'antd';
import { useFormContext, useWatch } from 'react-hook-form';

import { FormField } from '@/components/form/rhf';
import { useOutboundTags } from '@/api/queries/useOutboundTags';
import MtprotoWebStatus from './MtprotoWebStatus';
import { TelemtWebProxyRequestSchema } from '@/schemas/telemt-web';
import MtprotoMekoStatus from './MtprotoMekoStatus';

export default function MtprotoFields() {
  const { t, i18n } = useTranslation();
  const { control } = useFormContext();
  const webProxyEnabled = useWatch({ control, name: 'settings.webProxy.enabled' }) as
    | boolean
    | undefined;
  const mekoEnabled = useWatch({ control, name: 'settings.mekoFix' }) as boolean | undefined;
  const routeThroughXray = useWatch({ control, name: 'settings.routeThroughXray' }) as
    | boolean
    | undefined;
  const proxyProtocolEnabled = useWatch({ control, name: 'settings.proxyProtocolListener' });
  const legacyFronting = useWatch({ control, name: 'settings.domainFronting' });
  const { data: outboundTags } = useOutboundTags({ excludeBlackhole: true });
  const routeThroughXrayHint = i18n.resolvedLanguage?.startsWith('ru')
    ? 'Направляет Telegram-трафик этого Telemt inbound через локальный SOCKS-мост выбранного ядра Xray или Sing-box, чтобы применялись выбранное исходящее соединение и правила маршрутизации.'
    : 'Routes this Telemt inbound Telegram traffic through the local SOCKS bridge of the selected Xray or Sing-box core so the selected outbound and routing rules are applied.';

  return (
    <>
      <FormField
        name={['settings', 'webProxy', 'enabled']}
        label={`${t('menu.telemt')} · WEB Proxy (HTTPS)`}
        tooltip={
          i18n.resolvedLanguage?.startsWith('ru')
            ? 'Входящий MTProto создаёт и автоматически запускает WEB Proxy через HTTPS. Отдельно включать Telemt не требуется.'
            : 'This MTProto inbound automatically provisions the HTTPS WEB Proxy. No separate Telemt toggle is needed.'
        }
        valueProp="checked"
      >
        <Switch />
      </FormField>
      {webProxyEnabled && (
        <>
          <Alert
            type="info"
            showIcon
            message={
              i18n.resolvedLanguage?.startsWith('ru')
                ? 'WEB Proxy: внешний TCP-порт 443 — автоматически'
                : 'WEB Proxy: public TCP port 443 — automatic'
            }
            description={
              i18n.resolvedLanguage?.startsWith('ru')
                ? 'Можно использовать домен панели или отдельный домен. A/AAAA должны указывать на этот сервер. Панель автоматически настроит HTTPS frontend, сертификат и локальный backend. Если сама панель или другой сервис занимает 443, сначала перенесите его на другой порт; для панели и WEB допустим один домен с разными портами. Для выпуска и продления сертификата через HTTP нужен доступный снаружи TCP 80; для работы — TCP 443. Разрешите их в firewall сервера и хостера. Поддерживается один WEB inbound на локальном сервере.'
                : 'Use the panel domain or a separate domain with A/AAAA pointing to this server. HTTPS frontend, certificate and loopback backend are configured automatically. Move the panel or another service off port 443 first if necessary; the same domain can serve the panel on another port. HTTP certificate issuance and renewal need public TCP 80; proxy traffic needs TCP 443. Allow these ports in host and provider firewalls. One local WEB inbound is supported.'
            }
          />
          <MtprotoWebStatus />
          <FormField
            name={['settings', 'webProxy', 'domain']}
            label="Домен WEB Proxy"
            rules={{
              validate: (value) =>
                TelemtWebProxyRequestSchema.shape.domain.safeParse(value).success ||
                (i18n.resolvedLanguage?.startsWith('ru')
                  ? 'Укажите домен без https://, порта и пути, например web.example.com'
                  : 'Enter a domain without scheme, port or path, e.g. web.example.com'),
            }}
          >
            <Input placeholder="web.example.com" addonAfter=":443" />
          </FormField>
          <FormField name={['settings', 'webProxy', 'frontend']} label="HTTPS frontend">
            <Select
              options={['nginx', 'caddy', 'haproxy'].map((value) => ({ value, label: value }))}
            />
          </FormField>
          <FormField name={['settings', 'webProxy', 'carrier']} label="WEB carrier">
            <Select
              options={['https', 'https-lanes', 'websocket', 'websocket-lanes'].map((value) => ({
                value,
                label: value,
              }))}
            />
          </FormField>
          <FormField name={['settings', 'webProxy', 'carriers']} label="Приоритет транспортов">
            <Select
              mode="multiple"
              options={['https', 'https-lanes', 'websocket', 'websocket-lanes'].map((value) => ({
                value,
                label: value,
              }))}
            />
          </FormField>
        </>
      )}
      {!webProxyEnabled && (
        <>
          <FormField
            name={['settings', 'mekoFix']}
            label="Fix by MEKO V3"
            tooltip={
              i18n.resolvedLanguage?.startsWith('ru')
                ? 'Применить MEKO V3 только к TCP-порту этого обычного MTProto-прокси. На другие порты и WEB Proxy правила не распространяются.'
                : 'Apply MEKO V3 only to this ordinary MTProto inbound TCP port. Other ports and WEB Proxy are unaffected.'
            }
            valueProp="checked"
          >
            <Switch />
          </FormField>
          {mekoEnabled && (
            <>
              <MtprotoMekoStatus />
              <Collapse
                items={[
                  {
                    key: 'meko-limits',
                    label: i18n.resolvedLanguage?.startsWith('ru')
                      ? 'Лимиты защиты — настроены автоматически'
                      : 'Protection limits — configured automatically',
                    children: (
                      <>
                        <FormField
                          name={['settings', 'mekoRatePerMinute']}
                          label={
                            i18n.resolvedLanguage?.startsWith('ru')
                              ? 'Лимит SYN на IP (в минуту)'
                              : 'SYN limit per IP (per minute)'
                          }
                          tooltip={
                            i18n.resolvedLanguage?.startsWith('ru')
                              ? 'Для этого порта. Если пусто, используется общий лимит (обычно 54/мин). Слишком низкий лимит может затруднить подключение через NAT.'
                              : 'Only this port. If empty, use the global limit (normally 54/min). Low limits may prevent connections behind NAT.'
                          }
                        >
                          <InputNumber
                            min={1}
                            max={60000}
                            placeholder="54"
                            style={{ width: '100%' }}
                          />
                        </FormField>
                        <FormField
                          name={['settings', 'mekoBurst']}
                          label={
                            i18n.resolvedLanguage?.startsWith('ru')
                              ? 'Допустимый всплеск SYN (burst)'
                              : 'Allowed SYN burst'
                          }
                        >
                          <InputNumber
                            min={1}
                            max={1000}
                            placeholder="1"
                            style={{ width: '100%' }}
                          />
                        </FormField>
                        <FormField
                          name={['settings', 'mekoIosRatePerMinute']}
                          label={
                            i18n.resolvedLanguage?.startsWith('ru')
                              ? 'iOS SYN на IP (в минуту)'
                              : 'iOS SYN per IP (per minute)'
                          }
                        >
                          <InputNumber
                            min={1}
                            max={60000}
                            placeholder="900"
                            style={{ width: '100%' }}
                          />
                        </FormField>
                        <FormField
                          name={['settings', 'mekoIosBurst']}
                          label={
                            i18n.resolvedLanguage?.startsWith('ru')
                              ? 'Всплеск SYN для iOS'
                              : 'iOS SYN burst'
                          }
                        >
                          <InputNumber
                            min={1}
                            max={1000}
                            placeholder="30"
                            style={{ width: '100%' }}
                          />
                        </FormField>
                      </>
                    ),
                  },
                ]}
              />
            </>
          )}
          <FormField
            name={['settings', 'fakeTlsDomain']}
            label={t('pages.inbounds.form.fakeTlsDomain')}
            tooltip={t('pages.inbounds.form.mtprotoFakeTlsDomainHint')}
          >
            <Input placeholder="www.cloudflare.com" />
          </FormField>
          <FormField
            name={['settings', 'throttleMaxConnections']}
            label={t('pages.inbounds.form.mtgThrottleMaxConnections')}
            tooltip={t('pages.inbounds.form.mtgThrottleMaxConnectionsHint')}
          >
            <InputNumber min={0} placeholder="0" style={{ width: '100%' }} />
          </FormField>
          <FormField
            name={['settings', 'routeThroughXray']}
            label={t('pages.inbounds.form.mtgRouteThroughXray')}
            tooltip={routeThroughXrayHint}
            valueProp="checked"
          >
            <Switch />
          </FormField>
          {routeThroughXray && (
            <FormField
              name={['settings', 'outboundTag']}
              label={t('pages.inbounds.form.mtgRouteOutbound')}
              tooltip={t('pages.inbounds.form.mtgRouteOutboundHint')}
            >
              <Select
                id="mtprotoOutboundTag"
                allowClear
                showSearch
                placeholder={t('pages.inbounds.form.mtgRouteOutboundPlaceholder')}
                options={(outboundTags ?? []).map((tag) => ({ value: tag, label: tag }))}
              />
            </FormField>
          )}
          <Collapse
            items={[
              {
                key: 'advanced',
                label: t('pages.inbounds.form.telemtAdvanced'),
                children: (
                  <>
                    <FormField
                      name={['settings', 'allowLegacyModes']}
                      label={t('pages.inbounds.form.telemtLegacyModes')}
                      tooltip={t('pages.inbounds.form.telemtLegacyModesHint')}
                      valueProp="checked"
                      transform={{ input: (value) => value ?? true }}
                    >
                      <Switch />
                    </FormField>
                    <FormField
                      name={['settings', 'tlsMask']}
                      label={t('pages.inbounds.form.telemtTlsMask')}
                      tooltip={t('pages.inbounds.form.telemtTlsMaskHint')}
                      valueProp="checked"
                      transform={{ input: (value) => value ?? true }}
                    >
                      <Switch />
                    </FormField>
                    <FormField
                      name={['settings', 'tlsEmulation']}
                      label={t('pages.inbounds.form.telemtTlsEmulation')}
                      tooltip={t('pages.inbounds.form.telemtTlsEmulationHint')}
                      valueProp="checked"
                      transform={{ input: (value) => value ?? true }}
                    >
                      <Switch />
                    </FormField>
                    <FormField
                      name={['settings', 'unknownSniAction']}
                      label={t('pages.inbounds.form.telemtUnknownSniAction')}
                      transform={{ input: (value) => value ?? 'mask' }}
                    >
                      <Select
                        options={['mask', 'drop', 'accept', 'reject_handshake'].map((value) => ({
                          value,
                          label: t(`pages.inbounds.form.telemtSni_${value}`),
                        }))}
                      />
                    </FormField>
                    <FormField
                      name={['settings', 'proxyProtocolListener']}
                      label={t('pages.inbounds.form.mtgProxyProtocolListener')}
                      valueProp="checked"
                    >
                      <Switch />
                    </FormField>
                    {proxyProtocolEnabled && (
                      <FormField
                        name={['settings', 'proxyProtocolTrustedCidrs']}
                        label={t('pages.inbounds.form.telemtTrustedProxies')}
                        tooltip={t('pages.inbounds.form.telemtTrustedProxiesHint')}
                        transform={{ input: (value) => value ?? ['127.0.0.0/8', '::1/128'] }}
                      >
                        <Select mode="tags" tokenSeparators={[',', ' ']} />
                      </FormField>
                    )}
                    <FormField
                      name={['settings', 'preferIp']}
                      label={t('pages.inbounds.form.mtgPreferIp')}
                    >
                      <Select
                        allowClear
                        placeholder={t('pages.inbounds.form.telemtAutoIp')}
                        options={[
                          { value: 'prefer-ipv6', label: 'prefer-ipv6' },
                          { value: 'prefer-ipv4', label: 'prefer-ipv4' },
                          { value: 'only-ipv6', label: 'only-ipv6' },
                          { value: 'only-ipv4', label: 'only-ipv4' },
                        ]}
                      />
                    </FormField>
                    <FormField
                      name={['settings', 'debug']}
                      label={t('pages.inbounds.form.mtgDebug')}
                      valueProp="checked"
                    >
                      <Switch />
                    </FormField>
                    <FormField
                      name={['settings', 'publicIpv4']}
                      label={t('pages.inbounds.form.mtgPublicIpv4')}
                      tooltip={t('pages.inbounds.form.mtgPublicIpHint')}
                    >
                      <Input allowClear placeholder="1.2.3.4" />
                    </FormField>
                    <FormField
                      name={['settings', 'publicIpv6']}
                      label={t('pages.inbounds.form.mtgPublicIpv6')}
                      tooltip={t('pages.inbounds.form.mtgPublicIpHint')}
                    >
                      <Input allowClear placeholder="2001:db8::1" />
                    </FormField>
                    <FormField
                      name={['settings', 'maskHost']}
                      label={t('pages.inbounds.form.telemtMaskHost')}
                      tooltip={t('pages.inbounds.form.telemtMaskHostHint')}
                      transform={{ input: (value) => value ?? legacyFronting?.ip ?? '' }}
                    >
                      <Input allowClear placeholder="nginx.example.com" />
                    </FormField>
                    <FormField
                      name={['settings', 'maskPort']}
                      label={t('pages.inbounds.form.telemtMaskPort')}
                      transform={{ input: (value) => value ?? legacyFronting?.port ?? 443 }}
                    >
                      <InputNumber min={1} max={65535} style={{ width: '100%' }} />
                    </FormField>
                    <FormField
                      name={['settings', 'maskProxyProtocol']}
                      label={t('pages.inbounds.form.telemtMaskProxyProtocol')}
                      transform={{
                        input: (value) => value ?? (legacyFronting?.proxyProtocol ? 1 : 0),
                      }}
                    >
                      <Select
                        options={[
                          { value: 0, label: t('pages.inbounds.form.telemtDisabled') },
                          { value: 1, label: 'PROXY v1' },
                          { value: 2, label: 'PROXY v2' },
                        ]}
                      />
                    </FormField>
                  </>
                ),
              },
            ]}
          />
        </>
      )}
    </>
  );
}
