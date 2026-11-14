import { Button, Empty, Tag, Typography } from 'antd';
import {
  ApartmentOutlined,
  ArrowRightOutlined,
  GlobalOutlined,
  LockOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import type { DomainForm } from '@/schemas/domain-front';
import type { ManagedSniDomain } from '@/api/queries/useManagedSniDomains';
export default function DomainSniView({
  sites,
  onShare,
  pending,
  onEdit,
}: {
  sites: ManagedSniDomain[];
  pending: DomainForm[];
  onEdit: (name: string) => void;
  onShare: () => void;
}) {
  const { t, i18n } = useTranslation();
  const l = (k: string) => t('pages.settings.domains.' + k);
  const groups = new Map<number, ManagedSniDomain[]>();
  for (const site of sites) {
    const id = site.nodeId || 0;
    groups.set(id, [...(groups.get(id) || []), site]);
  }
  return (
    <section className="domain-sni-view" data-testid="domain-panel-sni">
      <div className="domain-section-heading">
        <div>
          <Typography.Title level={4}>{l('redesign.sniNodes')}</Typography.Title>
          <Typography.Text type="secondary">{l('redesign.nodesHint')}</Typography.Text>
        </div>
        <Button type="primary" onClick={onShare}>
          {l('workspace.takeNodeCertificate')}
        </Button>
      </div>
      <div className="domain-node-grid">
        {[...groups].map(([id, entries]) => (
          <article key={id} className="domain-node-card">
            <header>
              <ApartmentOutlined />
              <strong>{id ? entries[0].nodeName || `#${id}` : l('redesign.mainServer')}</strong>
              <Tag>{entries.length} SNI</Tag>
            </header>
            {entries.map((site) => (
              <div key={site.name + '-' + site.source} className="domain-node-identity">
                <div>
                  <GlobalOutlined />
                  <strong>{site.name}</strong>
                </div>
                <ArrowRightOutlined />
                <div>
                  <LockOutlined />
                  <span>
                    {site.expiresAt
                      ? new Intl.DateTimeFormat(i18n.language, { dateStyle: 'medium' }).format(
                          site.expiresAt * 1000,
                        )
                      : l('redesign.noCertificateData')}
                  </span>
                  <small>{site.issuer || site.source}</small>
                </div>
              </div>
            ))}
          </article>
        ))}
      </div>
      {pending.length > 0 && (
        <div className="domain-sni-pending">
          {pending.map((domain) => (
            <div key={domain.name}>
              <GlobalOutlined />
              <Typography.Text strong>{domain.name}</Typography.Text>
              <Tag>{l('workspace.health.pending')}</Tag>
              <Button size="small" onClick={() => onEdit(domain.name)}>
                {l('edit')}
              </Button>
            </div>
          ))}
        </div>
      )}
      {!sites.length && !pending.length && <Empty description={l('sniEmpty')} />}
    </section>
  );
}
