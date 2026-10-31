import { Select, Switch } from 'antd';
import { SettingListItem } from '@/components/ui';
import { object, records, setDefaultOutbound, type ConfigObject } from './simple-settings';
import { basicRouteFields, basicRule, setBasicRule } from './native-routing-basic';
import { nativeInboundRouteTarget, setNativeInboundRouteTarget } from './native-inbound-bindings';

export default function NativeRoutingBasic({
  config,
  onChange,
  subscriptionTags,
  inboundTags,
}: {
  config: ConfigObject;
  onChange: (config: ConfigObject) => void;
  subscriptionTags: string[];
  inboundTags: string[];
}) {
  const route = object(config.route);
  const tags = Array.from(
    new Set(
      [...records(config.outbounds), ...records(config.endpoints)]
        .map((row) => String(row.tag || ''))
        .filter(Boolean)
        .concat(subscriptionTags),
    ),
  );
  const direct = records(config.outbounds).find(
    (row) => row.type === 'direct' && typeof row.tag === 'string' && row.tag,
  );
  const torrent = basicRule(config, 'torrent');
  const availableInbounds = Array.from(
    new Set(
      [
        ...records(config.inbounds).map((inbound) => String(inbound.tag || '')),
        ...inboundTags,
      ].filter(Boolean),
    ),
  );
  return (
    <div>
      <SettingListItem
        title="Выход по умолчанию"
        description="Для трафика, который не совпал ни с одним правилом."
      >
        <Select
          aria-label="Выход в интернет"
          style={{ width: '100%' }}
          value={String(route.final || '')}
          options={[
            { value: '', label: 'Первое исходящее подключение' },
            ...tags.map((value) => ({ value, label: value })),
          ]}
          onChange={(value) => onChange(setDefaultOutbound(config, value))}
        />
      </SettingListItem>
      {availableInbounds.length > 0 && (
        <section aria-label="Привязка входящих к исходящим">
          <div style={{ fontWeight: 600, fontSize: 16, margin: '20px 0 6px' }}>
            Привязка входящих к исходящим
          </div>
          <p style={{ opacity: 0.75, marginBottom: 12 }}>
            Для каждого входящего можно назначить отдельный выход. Предшествующие правила
            маршрутизации сохраняют приоритет.
          </p>
          {availableInbounds.map((tag) => (
            <SettingListItem key={tag} title={tag}>
              <Select
                aria-label={`Исходящее для ${tag}`}
                allowClear
                placeholder="Использовать маршрут по умолчанию"
                style={{ width: '100%' }}
                value={nativeInboundRouteTarget(config, tag) || undefined}
                options={tags.map((value) => ({ value, label: value }))}
                onChange={(value: string | undefined) =>
                  onChange(setNativeInboundRouteTarget(config, tag, value || ''))
                }
              />
            </SettingListItem>
          ))}
        </section>
      )}
      <SettingListItem title="Блокировать BitTorrent">
        <Switch
          checked={
            torrent.enabled !== false &&
            Array.isArray(torrent.protocol) &&
            torrent.protocol.includes('bittorrent')
          }
          onChange={(enabled) =>
            onChange(
              setBasicRule(config, 'torrent', {
                action: 'reject',
                protocol: enabled ? ['bittorrent'] : [],
              }),
            )
          }
        />
      </SettingListItem>
      {basicRouteFields.map((setting) => {
        const rule = basicRule(config, setting.id);
        const values = Array.isArray(rule[setting.field]) ? (rule[setting.field] as string[]) : [];
        return (
          <SettingListItem
            key={setting.id}
            title={setting.title}
            description={
              setting.action === 'route' && !direct
                ? 'Сначала добавьте исходящее подключение типа direct.'
                : 'Добавьте значения через Enter. Пустой список отключает это правило.'
            }
          >
            <Select
              mode="tags"
              aria-label={setting.title}
              style={{ width: '100%' }}
              disabled={setting.action === 'route' && !direct}
              value={values}
              tokenSeparators={[',', '\n']}
              onChange={(entries: string[]) =>
                onChange(
                  setBasicRule(config, setting.id, {
                    action: setting.action,
                    ...(setting.action === 'route' ? { outbound: direct?.tag } : {}),
                    [setting.field]: entries.map((entry) => entry.trim()).filter(Boolean),
                  }),
                )
              }
            />
          </SettingListItem>
        );
      })}
    </div>
  );
}
