import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Card, Empty, Pagination, Skeleton, Space, Tag, Typography } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { HttpUtil } from '@/utils';
import type { RemoteTelegramProxy } from '@/generated/types';

export default function RemoteTelegramProxies() {
  const { t } = useTranslation();
  const [page, setPage] = useState(1);
  const [profiles, setProfiles] = useState<RemoteTelegramProxy[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);
  const [revision, setRevision] = useState(0);
  const refresh = useCallback(() => {
    setLoading(true);
    setRevision((value) => value + 1);
  }, []);
  useEffect(() => {
    let active = true;
    void HttpUtil.get<RemoteTelegramProxy[]>('/panel/api/telemt/remote-proxies', undefined, {
      silent: true,
    })
      .then((response) => {
        if (!active) return;
        if (!response?.success || !Array.isArray(response.obj)) {
          setError(true);
          return;
        }
        setProfiles(response.obj);
        setPage(1);
        setError(false);
      })
      .catch(() => {
        if (active) setError(true);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [revision]);
  return (
    <Card
      className="telemt-card telemt-remote-card"
      title={t('pages.telemt.remoteTitle')}
      extra={
        <Button
          icon={<ReloadOutlined aria-hidden />}
          aria-label={t('pages.telemt.remoteRefresh')}
          loading={loading}
          onClick={refresh}
        />
      }
    >
      <Typography.Paragraph type="secondary">{t('pages.telemt.remoteHint')}</Typography.Paragraph>
      {error && <Alert type="warning" showIcon title={t('pages.telemt.remoteError')} />}
      {loading ? (
        <Skeleton active />
      ) : profiles.length === 0 ? (
        <Empty description={t('pages.telemt.remoteEmpty')} />
      ) : (
        <>
          <div className="telemt-proxy-list">
            {profiles.slice((page - 1) * 12, page * 12).map(({ nodeId, nodeName, profile }) => (
              <div className="telemt-proxy-item" key={`${nodeId}:${profile.link}`}>
                <div className="telemt-proxy-main">
                  <Space wrap>
                    <Tag>{nodeName}</Tag>
                    <Typography.Text strong>{profile.name || profile.host}</Typography.Text>
                  </Space>
                  <Typography.Paragraph
                    copyable={{ text: profile.link }}
                    ellipsis={{ rows: 1 }}
                    style={{ marginBottom: 0 }}
                  >
                    <bdi>
                      {profile.host}
                      {profile.port ? `:${profile.port}` : ''}
                    </bdi>
                  </Typography.Paragraph>
                </div>
                <Button href={profile.link}>{t('subscription.openTelegram')}</Button>
              </div>
            ))}
          </div>
          <Pagination
            current={page}
            pageSize={12}
            total={profiles.length}
            onChange={setPage}
            hideOnSinglePage
            showSizeChanger={false}
            style={{ marginTop: 16 }}
          />
        </>
      )}
    </Card>
  );
}
