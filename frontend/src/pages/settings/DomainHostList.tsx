import { useState } from 'react';
import { Button, Dropdown, Empty, Input, Pagination, Select, Space, Tag, Typography } from 'antd';
import {
  GlobalOutlined,
  LockOutlined,
  MoreOutlined,
  ArrowRightOutlined,
  CloudServerOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import type { DomainForm, DomainConfigForm } from '@/schemas/domain-front';
import type { DomainFrontStatus, DomainFrontCheck } from '@/generated/zod';
import type { ManagedSniDomain } from '@/api/queries/useManagedSniDomains';
import { getDomainHealth } from './domain-health';
export default function DomainHostList({
  domains,
  draft,
  status,
  sites,
  checks,
  search,
  filter,
  onSearch,
  onFilter,
  onEdit,
  onRemove,
  onCheck,
  checking,
  disabled,
  expanded,
}: {
  domains: DomainForm[];
  draft: DomainConfigForm;
  status?: DomainFrontStatus;
  sites: ManagedSniDomain[];
  checks: Record<string, DomainFrontCheck>;
  search: string;
  filter: string;
  onSearch: (s: string) => void;
  onFilter: (s: string) => void;
  onEdit: (name: string) => void;
  onRemove: (name: string) => void;
  onCheck: (name: string) => void;
  checking?: string;
  disabled: boolean;
  expanded: string[];
}) {
  const { t } = useTranslation();
  const l = (k: string) => t('pages.settings.domains.' + k);
  const [requestedPage, setPage] = useState(1);
  const page = Math.min(requestedPage, Math.max(1, Math.ceil(domains.length / 8)));
  const mode = (value: string) =>
    value === 'cert_only' ? l('redesign.certificateOnly') : l('mode.' + value);
  return (
    <section className="domain-host-workspace" aria-label={l('list')}>
      <div className="domain-host-toolbar">
        <Input.Search
          allowClear
          placeholder={l('workspace.searchPlaceholder')}
          aria-label={l('workspace.searchPlaceholder')}
          value={search}
          onChange={(e) => onSearch(e.target.value)}
        />
        <Select
          aria-label={l('workspace.filterLabel')}
          value={filter}
          onChange={onFilter}
          options={[
            { value: 'all', label: l('workspace.filter.all') },
            { value: 'attention', label: l('workspace.filter.attention') },
            ...[
              'direct',
              'cdn',
              'relay',
              'sub_link_only',
              'sni_only',
              'shared_sni',
              'cert_only',
            ].map((value) => ({ value, label: mode(value) })),
          ]}
        />
        <Typography.Text type="secondary">
          {domains.length} / {draft.domains.length}
        </Typography.Text>
      </div>
      <div className="domain-host-columns" aria-hidden>
        <span>{l('name')}</span>
        <span>{l('redesign.destination')}</span>
        <span>{l('redesign.certificate')}</span>
        <span>{l('redesign.server')}</span>
        <span />
      </div>
      <div className="domain-config-list" role="list">
        {domains.slice((page - 1) * 8, page * 8).map((domain) => {
          const saved =
            status?.config.domains.some(
              (d) => d.name === domain.name && JSON.stringify(d) === JSON.stringify(domain),
            ) ?? false;
          const health = getDomainHealth(domain, status, checks[domain.name], saved);
          const cert = status?.certificates.find((c) => c.name === domain.name);
          const remote = domain.mode === 'shared_sni';
          const node = sites.find((s) => s.name === domain.name && s.source === 'remote');
          const result = checks[domain.name];
          return (
            <article
              key={domain.name}
              role="listitem"
              className={'domain-host-row' + (!domain.enabled ? ' is-disabled' : '')}
            >
              <div className="domain-host-flow">
                <div className="domain-host-identity">
                  <GlobalOutlined />
                  <div>
                    <Typography.Text strong className="domain-host-name">
                      {domain.name}
                    </Typography.Text>
                    <Space size={3} wrap>
                      {draft.subscriptionDomain === domain.name && (
                        <Tag color="blue">{l('workspace.subscriptionDomain')}</Tag>
                      )}
                      {!saved && <Tag color="orange">{l('state.unsaved')}</Tag>}
                      {!domain.enabled && <Tag>{l('disabled')}</Tag>}
                    </Space>
                  </div>
                </div>
                <div className="domain-host-target">
                  <span className="domain-mobile-label">{l('redesign.destination')}</span>
                  <span>
                    <ArrowRightOutlined /> {mode(domain.mode)}
                  </span>
                  <Typography.Text type="secondary">
                    {domain.mode === 'relay'
                      ? domain.origin
                      : domain.inboundIds.length
                        ? t('pages.settings.domains.redesign.inboundCount', {
                            count: domain.inboundIds.length,
                          })
                        : domain.mode === 'sni_only'
                          ? domain.siteCatalog === 'custom'
                            ? l('redesign.customSite')
                            : domain.siteTemplate || l('redesign.coverSite')
                          : domain.mode === 'cert_only'
                            ? 'DNS-01'
                            : l('redesign.noBindings')}
                  </Typography.Text>
                </div>
                <div className="domain-host-certificate">
                  <span className="domain-mobile-label">{l('redesign.certificate')}</span>
                  <span>
                    <LockOutlined />{' '}
                    <Tag
                      color={
                        health === 'healthy'
                          ? 'success'
                          : health === 'error'
                            ? 'error'
                            : health === 'expiring'
                              ? 'warning'
                              : 'default'
                      }
                    >
                      {l('workspace.health.' + health)}
                    </Tag>
                  </span>
                  <Typography.Text type="secondary">
                    {cert?.expiresAt
                      ? `${cert.daysLeft} ${l('days')} · ${cert.autoRenew ? l('redesign.autoRenewShort') : l('redesign.externalRenewal')}`
                      : remote
                        ? l('remoteSNITag')
                        : l('tls.' + domain.tls)}
                  </Typography.Text>
                </div>
                <div className="domain-host-server">
                  <span className="domain-mobile-label">{l('redesign.server')}</span>
                  <span>
                    <CloudServerOutlined />{' '}
                    {remote ? node?.nodeName || l('remoteSNITag') : l('redesign.mainServer')}
                  </span>
                  <Typography.Text type="secondary">
                    {remote
                      ? l('redesign.remoteIdentity')
                      : domain.mode === 'cert_only'
                        ? 'DNS-01'
                        : `HTTPS · ${draft.httpsPort}`}
                  </Typography.Text>
                </div>
                <div className="domain-host-buttons">
                  <Button size="small" onClick={() => onEdit(domain.name)}>
                    {l('edit')}
                  </Button>
                  <Dropdown
                    trigger={['click']}
                    menu={{
                      items: [
                        { key: 'check', label: l('check'), disabled },
                        { type: 'divider' },
                        { key: 'remove', label: l('remove'), danger: true },
                      ],
                      onClick: ({ key }) => {
                        if (key === 'check') onCheck(domain.name);
                        if (key === 'remove') onRemove(domain.name);
                      },
                    }}
                  >
                    <Button
                      size="small"
                      icon={<MoreOutlined />}
                      aria-label={l('workspace.moreActions')}
                      loading={checking === domain.name}
                    />
                  </Dropdown>
                </div>
              </div>
              {result && expanded.includes(domain.name) && (
                <div className="domain-host-diagnostics">
                  <Typography.Text>
                    {l('dns')}: {result.addresses.join(', ') || '—'}
                  </Typography.Text>
                  {result.expiresAt > 0 && (
                    <Typography.Text>
                      {l('publicTLS')}: {result.issuer} · {result.daysLeft} {l('days')}
                    </Typography.Text>
                  )}
                  {result.error && <Typography.Text type="danger">{result.error}</Typography.Text>}
                  <Typography.Text type="secondary">{l('publicTLSDesc')}</Typography.Text>
                </div>
              )}
            </article>
          );
        })}
      </div>
      {!domains.length && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={l('empty')} />}
      {domains.length > 8 && (
        <Pagination
          current={page}
          pageSize={8}
          total={domains.length}
          onChange={setPage}
          showSizeChanger={false}
        />
      )}
    </section>
  );
}
