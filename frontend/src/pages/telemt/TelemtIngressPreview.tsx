import { Alert, Button, Descriptions, Space, Typography } from 'antd';
import { useTranslation } from 'react-i18next';
import { TelemtPortMigrationButton } from './TelemtPortMigration';

export type VPNMigrationRequest = { id: number; newPort: number; revision: string };

type VPNMigration = {
  revision?: string;
  security?: string;
  vetroffH3Bind?: string;
  vetroffPinH3?: boolean;
  trustTunnelQuicBind?: string;
  id: number;
  name: string;
  protocol: string;
  oldListen: string;
  oldPort: number;
  newListen?: string;
  newPort?: number;
  sni?: string;
  public?: string;
  reason?: string;
};

export type IngressPreview = {
  vpnMigrations?: VPNMigration[];
  routes: { name: string; public: string; backend: string; tls: boolean }[];
  suggestedAliases: string[];
  suggestedBasePath?: string;
  certificateAction: 'reuse' | 'issue-http01' | 'san-required';
  panelPort: number;
  subscriptionPort?: number;
  telemtPort?: number;
  suggestedPanelPort?: number;
  suggestedSubscriptionPort?: number;
  manual: string[];
};

export default function TelemtIngressPreview({
  preview,
  aliases,
  basePath,
  onAccept,
  disabled,
  allowPortMigration,
  onApplyVPNMigration,
  applyVPNMigrationLabel,
}: {
  preview: IngressPreview;
  aliases: string[];
  basePath: string;
  onAccept: (aliases: string[], basePath?: string) => void;
  disabled?: boolean;
  allowPortMigration?: boolean;
  onApplyVPNMigration?: (moves: VPNMigrationRequest[]) => void;
  applyVPNMigrationLabel?: string;
}) {
  const { i18n } = useTranslation();
  const ru = i18n.resolvedLanguage?.startsWith('ru') ?? false;
  const moves = preview.vpnMigrations ?? [];
  const canMigrate =
    moves.length > 0 &&
    moves.every(
      (item) =>
        !item.reason &&
        (item.security === 'tls' || (item.security === 'reality' && item.protocol === 'vless')) &&
        item.newPort &&
        item.revision,
    );
  const newAliases = preview.suggestedAliases.filter((host) => !aliases.includes(host));
  const nextPath = preview.suggestedBasePath;
  const pathChanged = Boolean(nextPath && nextPath !== basePath.replace(/^\/+|\/+$/g, ''));
  const certificate = {
    reuse: ru ? 'Использовать существующий сертификат' : 'Reuse existing certificate',
    'issue-http01': ru ? 'Автоматический выпуск HTTP-01' : 'Automatic HTTP-01 issuance',
    'san-required': ru
      ? 'Требуется сертификат SAN/wildcard для всех доменов'
      : 'SAN/wildcard certificate required for all domains',
  }[preview.certificateAction];
  const manualMessage = (code: string) => {
    const messages: Record<string, [string, string]> = {
      'panel-env-port': [
        'XUI_PORT=443: сначала измените переменную окружения службы панели.',
        'XUI_PORT=443: change the service environment first.',
      ],
      'subscription-url': [
        'Публичный URL подписок должен использовать HTTPS без отдельного порта или пути.',
        'Subscription public URL must use HTTPS without a custom port or path.',
      ],
      'web-path': [
        'Не найден свободный WEB-путь. Проверьте пути панели и подписок.',
        'No non-overlapping WEB path found. Review panel and subscription paths.',
      ],
      'panel-tls': [
        'Для локальной панели необходимы действующие TLS-сертификат и ключ.',
        'The local panel requires a valid TLS certificate and key.',
      ],
      'panel-port': [
        'Перенесите панель на внутренний TCP-порт в Настройки → Панель и переподключитесь.',
        'Move the panel to an internal TCP port in Settings → Panel, then reconnect.',
      ],
      'subscription-port': [
        'Перенесите подписки на отдельный внутренний TCP-порт в Настройки → Подписки.',
        'Move subscriptions to an internal TCP port in Settings → Subscriptions.',
      ],
      'port-unavailable': [
        'Не удалось подобрать свободный внутренний порт. Выберите его вручную и повторите проверку.',
        'No unreserved internal port found. Choose one manually and recheck.',
      ],
    };
    if (code.startsWith('required-host:'))
      return (
        (ru
          ? 'Не удалось добавить обязательный WEB-домен: '
          : 'Cannot add required WEB hostname: ') + code.slice('required-host:'.length)
      );
    return messages[code]?.[ru ? 0 : 1] ?? code;
  };
  const vpnMigrationReason = (reason: string) => {
    const messages: Record<string, [string, string]> = {
      disabled: [
        'Подключение отключено, но порт зарезервирован.',
        'Inbound is disabled but reserves the port.',
      ],
      'dependent-listener': [
        'Необходимо проверить зависимые слушатели и fallback.',
        'Dependent listeners and fallbacks require review.',
      ],
      'udp-or-mixed': [
        'UDP или смешанный транспорт нельзя переносить TCP-маршрутом.',
        'UDP or mixed transport cannot be moved through a TCP route.',
      ],
      'unsupported-protocol': [
        'Для этого протокола нужен отдельный механизм переноса.',
        'This protocol requires a separate migration mechanism.',
      ],
      'invalid-settings': [
        'Некорректная конфигурация подключения.',
        'Invalid inbound configuration.',
      ],
      wrapper: [
        'Дополнительная обёртка требует отдельной проверки.',
        'Additional wrapper requires separate validation.',
      ],
      'specific-address': [
        'Подключение привязано к отдельному IP-адресу.',
        'Inbound is bound to a specific IP address.',
      ],
      'tls-sni': [
        'Нужен поддерживаемый TLS/REALITY-транспорт с одним корректным SNI.',
        'A supported TLS/REALITY transport with a single valid SNI is required.',
      ],
      'web-sni': ['SNI совпадает с WEB-доменом.', 'SNI conflicts with a WEB hostname.'],
      'vetroff-h3': [
        'Нужны явные параметры HTTP/3 VetrOFF и корректный UDP-адрес.',
        'Explicit VetrOFF HTTP/3 settings and a valid UDP bind are required.',
      ],
      'shadowtls-client-required': [
        'Нужен активный ShadowTLS v3 клиент без лимитов и действующий внутренний ключ.',
        'An enabled unlimited ShadowTLS v3 client and a valid inner key are required.',
      ],
      'trusttunnel-quic-only-unsupported': [
        'Установленный TrustTunnel не прошёл проверку независимого UDP/QUIC-слушателя. Оригинальные подключения не изменены. Нужен совместимый endpoint либо явный TCP-only режим.',
        'The installed TrustTunnel failed the independent UDP/QUIC listener check. Existing connections remain unchanged. A compatible endpoint or explicit TCP-only mode is required.',
      ],
      'trusttunnel-shared-bind': [
        'TrustTunnel использует TCP и QUIC на одном адресе. Автоперенос доступен только после явного выбора TCP-режима и сохранения подключения.',
        'TrustTunnel shares its TCP and QUIC bind. Select and save TCP-only mode explicitly before automatic migration.',
      ],
      'reality-client-required': [
        'Для проверки нужен активный VLESS REALITY-клиент без лимита трафика и IP, с совместимым профилем.',
        'An enabled VLESS REALITY client with a compatible profile and no traffic or IP limit is required.',
      ],
      'reality-verification': [
        'Для REALITY ещё нужна проверка авторизованного клиента; автоматический перенос отключён.',
        'REALITY needs an authenticated client probe; automatic migration is disabled.',
      ],
      'duplicate-sni': [
        'SNI повторяется у другого подключения.',
        'Another inbound uses the same SNI.',
      ],
      'port-unavailable': [
        'Не удалось подобрать внутренний TCP-порт.',
        'No suitable internal TCP port found.',
      ],
    };
    return messages[reason]?.[ru ? 0 : 1] ?? reason;
  };
  return (
    <Space orientation="vertical" style={{ width: '100%' }}>
      <Typography.Text strong>
        {ru ? 'План настройки (без изменений на сервере)' : 'Setup preview (no server changes)'}
      </Typography.Text>
      <Descriptions size="small" column={1} bordered>
        <Descriptions.Item label={ru ? 'Порт панели' : 'Panel port'}>
          {preview.panelPort || '—'}
          {preview.suggestedPanelPort
            ? ` → ${preview.suggestedPanelPort} (${ru ? 'вручную' : 'manual'})`
            : ''}
        </Descriptions.Item>
        {preview.subscriptionPort !== undefined && (
          <Descriptions.Item label={ru ? 'Порт подписок' : 'Subscription port'}>
            {preview.subscriptionPort || '—'}
            {preview.suggestedSubscriptionPort
              ? ` → ${preview.suggestedSubscriptionPort} (${ru ? 'вручную' : 'manual'})`
              : ''}
          </Descriptions.Item>
        )}
        <Descriptions.Item label={ru ? 'Внутренний порт Telemt' : 'Internal Telemt port'}>
          {preview.telemtPort || '—'}
        </Descriptions.Item>
        <Descriptions.Item label="TLS">{certificate}</Descriptions.Item>
        {newAliases.length > 0 && (
          <Descriptions.Item label={ru ? 'Добавить WEB-домены' : 'Add WEB hostnames'}>
            {newAliases.join(', ')}
          </Descriptions.Item>
        )}
        {pathChanged && (
          <Descriptions.Item label={ru ? 'WEB-путь' : 'WEB path'}>/{nextPath}</Descriptions.Item>
        )}
      </Descriptions>
      {(newAliases.length > 0 || pathChanged) && (
        <Button disabled={disabled} onClick={() => onAccept(preview.suggestedAliases, nextPath)}>
          {ru ? 'Подставить рекомендации в форму' : 'Fill recommended form values'}
        </Button>
      )}
      {allowPortMigration && <TelemtPortMigrationButton preview={preview} disabled={disabled} />}
      {Boolean(preview.vpnMigrations?.length) && (
        <>
          <Typography.Text strong>
            {ru ? 'Подключения, резервирующие TCP 443' : 'Inbounds reserving TCP 443'}
          </Typography.Text>
          <Alert
            type="info"
            showIcon
            title={
              ru
                ? 'Это предварительный план. VPN ещё не переносится; действующие подписки не меняются.'
                : 'This is a preview. VPN listeners and existing subscriptions are not changed.'
            }
          />
          <Descriptions size="small" column={1} bordered>
            {preview.vpnMigrations?.map((item) => (
              <Descriptions.Item
                key={item.id}
                label={item.name + ' · ' + item.protocol + ' · #' + item.id}
              >
                {item.reason ? (
                  <Space orientation="vertical" size={0}>
                    <Typography.Text type="warning">
                      {vpnMigrationReason(item.reason)}
                    </Typography.Text>
                    {item.trustTunnelQuicBind && (
                      <Typography.Text type="secondary">
                        {ru ? 'Текущий QUIC-адрес' : 'Current QUIC bind'}:{' '}
                        {item.trustTunnelQuicBind}
                      </Typography.Text>
                    )}
                  </Space>
                ) : (
                  <Space orientation="vertical" size={0}>
                    <Typography.Text copyable={{ text: String(item.newPort) }}>
                      {item.oldListen || '*'}:{item.oldPort} → {item.newListen}:{item.newPort}
                    </Typography.Text>
                    <Typography.Text type="secondary">
                      {item.public} · SNI: {item.sni}
                    </Typography.Text>
                    {item.trustTunnelQuicBind && (
                      <Typography.Text type="secondary">
                        QUIC · UDP {item.trustTunnelQuicBind} ({ru ? 'сохраняется' : 'preserved'})
                      </Typography.Text>
                    )}
                    {item.vetroffH3Bind && (
                      <Typography.Text type="secondary">
                        HTTP/3 · UDP {item.vetroffH3Bind} ({ru ? 'сохраняется' : 'preserved'})
                      </Typography.Text>
                    )}
                  </Space>
                )}
              </Descriptions.Item>
            ))}
          </Descriptions>
          {onApplyVPNMigration && canMigrate && (
            <Button
              danger
              disabled={disabled}
              onClick={() =>
                onApplyVPNMigration(
                  moves.map((item) => ({
                    id: item.id,
                    newPort: item.newPort!,
                    revision: item.revision!,
                  })),
                )
              }
            >
              {applyVPNMigrationLabel ??
                (ru ? 'Перенести VPN и запустить HTTPS-вход' : 'Move VPN and enable HTTPS entry')}
            </Button>
          )}
        </>
      )}
      {preview.manual.map((warning) => (
        <Alert key={warning} type="warning" showIcon title={manualMessage(warning)} />
      ))}
      {preview.routes.length > 0 && (
        <Descriptions size="small" column={1} bordered>
          {preview.routes.map((route) => (
            <Descriptions.Item key={route.name + route.public} label={route.name}>
              <Typography.Text style={{ overflowWrap: 'anywhere' }}>{route.public}</Typography.Text>
              <div>
                <Typography.Text type="secondary">
                  → {route.backend} · {route.tls ? 'TLS' : 'HTTP'}
                </Typography.Text>
              </div>
            </Descriptions.Item>
          ))}
        </Descriptions>
      )}
    </Space>
  );
}
