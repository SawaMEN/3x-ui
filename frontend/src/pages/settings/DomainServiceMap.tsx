import { Tag, Typography } from 'antd';
import {
  DesktopOutlined,
  LinkOutlined,
  ApartmentOutlined,
  ArrowRightOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import type { AllSetting } from '@/models/setting';
import type { DomainConfigForm } from '@/schemas/domain-front';
import type { ManagedSniDomain } from '@/api/queries/useManagedSniDomains';
export default function DomainServiceMap({
  settings,
  draft,
  sites,
  onPanel,
  onSubscription,
  subscriptionDisabled = false,
  onNodes,
}: {
  settings?: AllSetting;
  draft: DomainConfigForm;
  sites: ManagedSniDomain[];
  subscriptionDisabled?: boolean;
  onPanel: () => void;
  onSubscription: () => void;
  onNodes: () => void;
}) {
  const { t } = useTranslation();
  const l = (k: string) => t('pages.settings.domains.redesign.' + k);
  const tiles = [
    {
      key: 'panel',
      icon: <DesktopOutlined />,
      title: l('panel'),
      value: settings?.webDomain || l('byAddress'),
      detail: settings
        ? `${settings.webCertFile && settings.webKeyFile ? 'HTTPS' : 'HTTP'} · ${settings.webPort}`
        : l('unavailable'),
      onClick: onPanel,
    },
    {
      key: 'subscription',
      icon: <LinkOutlined />,
      title: l('subscription'),
      value: draft.subscriptionDomain || l('notAssigned'),
      detail: draft.subscriptionDomain ? `HTTPS · ${draft.publicPort}` : l('chooseDomain'),
      onClick: onSubscription,
      disabled: subscriptionDisabled,
    },
    {
      key: 'nodes',
      icon: <ApartmentOutlined />,
      title: l('sniNodes'),
      value: l('identities') + ': ' + new Set(sites.map((s) => s.name)).size,
      detail: l('servers') + ': ' + new Set(sites.map((s) => s.nodeId || 0)).size,
      onClick: onNodes,
    },
  ];
  return (
    <section className="domain-service-map" aria-label={l('serviceMap')}>
      {tiles.map((tile) => (
        <button
          type="button"
          key={tile.key}
          className="domain-service-tile"
          onClick={tile.onClick}
          disabled={tile.disabled}
        >
          <span className="domain-service-tile-top">
            <span className="domain-service-icon">{tile.icon}</span>
            <span>{tile.title}</span>
            <ArrowRightOutlined className="domain-tile-arrow" />
          </span>
          <strong title={tile.value}>{tile.value}</strong>
          <span className="domain-service-tile-bottom">
            <Typography.Text type="secondary">{tile.detail}</Typography.Text>
            <Tag>{l('configure')}</Tag>
          </span>
        </button>
      ))}
    </section>
  );
}
