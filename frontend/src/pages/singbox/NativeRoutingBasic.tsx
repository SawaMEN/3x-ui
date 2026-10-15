import { Select, Switch } from 'antd';
import { SettingListItem } from '@/components/ui';
import { object, records, setDefaultOutbound, type ConfigObject } from './simple-settings';
import { basicRouteFields, basicRule, setBasicRule } from './native-routing-basic';

export default function NativeRoutingBasic({
  config,
  onChange,
  subscriptionTags,
}: {
  config: ConfigObject;
  onChange: (config: ConfigObject) => void;
  subscriptionTags: string[];
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
