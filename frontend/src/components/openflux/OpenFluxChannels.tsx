import { useId, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Collapse,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Tag,
  Typography,
} from 'antd';
import {
  ApiOutlined,
  DeleteOutlined,
  FileTextOutlined,
  PlusOutlined,
  VideoCameraOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { onNumber } from '@/utils/onNumber';
import { HttpUtil } from '@/utils';
import {
  OPENFLUX_TRANSPORTS,
  openFluxTransportLabel,
  openFluxChannelValid,
  suggestOpenFluxTransport,
  validOpenFluxChannelURL,
  validOpenFluxMAXToken,
  validOpenFluxMAXUID,
  type OpenFluxTransport,
  type OpenFluxTransportType,
} from '@/lib/openflux';
import './OpenFluxChannels.css';

function ChannelInputs({
  value,
  change,
  client,
}: {
  value: OpenFluxTransport;
  change: (patch: Partial<OpenFluxTransport>) => void;
  client: boolean;
}) {
  const { t } = useTranslation();
  const uid = useId();
  if (value.type === 'direct')
    return client ? (
      <Form.Item
        label={t('pages.inbounds.form.openfluxEndpoint')}
        tooltip={t('pages.inbounds.form.openfluxEndpointHint')}
        htmlFor={`${uid}-dial`}
      >
        <Input
          id={`${uid}-dial`}
          value={value.dial ?? ''}
          placeholder="vpn.example.org:443"
          onChange={(e) => change({ dial: e.target.value })}
        />
      </Form.Item>
    ) : (
      <Typography.Paragraph type="secondary" className="openflux-channel-note">
        {t('pages.inbounds.form.openfluxDirectAuto')}
      </Typography.Paragraph>
    );
  if (value.type === 'oneme')
    return (
      <div className="openflux-account-fields">
        <Form.Item
          label={t(
            client
              ? 'pages.inbounds.form.openfluxMaxClientToken'
              : 'pages.inbounds.form.openfluxMaxServerToken',
          )}
          htmlFor={`${uid}-token`}
          tooltip={t('pages.inbounds.form.openfluxMaxTokenHelp')}
          validateStatus={value.token && !validOpenFluxMAXToken(value.token) ? 'error' : undefined}
          help={
            value.token && !validOpenFluxMAXToken(value.token)
              ? t('pages.inbounds.form.openfluxMaxTokenInvalid')
              : undefined
          }
        >
          <Input.Password
            id={`${uid}-token`}
            autoComplete="new-password"
            value={value.token ?? ''}
            onChange={(e) => change({ token: e.target.value.trim() })}
          />
        </Form.Item>
        <Form.Item
          label={t('pages.inbounds.form.openfluxMaxServerID')}
          htmlFor={`${uid}-uid`}
          tooltip={t(
            client
              ? 'pages.inbounds.form.openfluxMaxPeerHelp'
              : 'pages.inbounds.form.openfluxMaxIDHelp',
          )}
          validateStatus={value.uid && !validOpenFluxMAXUID(value.uid) ? 'error' : undefined}
          help={
            value.uid && !validOpenFluxMAXUID(value.uid)
              ? t('pages.inbounds.form.openfluxMaxIDInvalid')
              : undefined
          }
        >
          <Input
            id={`${uid}-uid`}
            inputMode="numeric"
            value={value.uid ?? ''}
            onChange={(e) => change({ uid: e.target.value.trim() })}
          />
        </Form.Item>
        <Typography.Paragraph type="secondary" className="openflux-channel-note">
          {t('pages.inbounds.form.openfluxMaxAccounts')}{' '}
          <Typography.Link href="https://web.max.ru" target="_blank" rel="noopener noreferrer">
            Web MAX ↗
          </Typography.Link>
        </Typography.Paragraph>
      </div>
    );
  return (
    <Form.Item
      label={t('pages.inbounds.form.openfluxChannelURL')}
      htmlFor={`${uid}-url`}
      tooltip={t('pages.inbounds.form.openfluxChannelHint')}
      validateStatus={value.url && !validOpenFluxChannelURL(value.url) ? 'error' : undefined}
      help={
        value.url && !validOpenFluxChannelURL(value.url)
          ? t('pages.inbounds.form.openfluxURLRequired')
          : undefined
      }
    >
      <Input
        id={`${uid}-url`}
        value={value.url ?? ''}
        placeholder={t(`pages.inbounds.form.openfluxPlaceholders.${value.type}`)}
        onChange={(e) => change({ url: e.target.value.trim() })}
      />
    </Form.Item>
  );
}

export default function OpenFluxChannels({
  value,
  onChange,
  client = false,
  nodeId = 0,
}: {
  value: OpenFluxTransport[];
  onChange: (value: OpenFluxTransport[]) => void;
  client?: boolean;
  nodeId?: number;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<OpenFluxTransport | null>(null);
  const [checking, setChecking] = useState<OpenFluxTransportType | null>(null);
  const [checks, setChecks] = useState<Record<string, { ok: boolean; detail: string }>>({});
  const used = new Set(value.map((entry) => entry.type));
  const available = OPENFLUX_TRANSPORTS.filter((type) => !used.has(type));
  const patch = (index: number, next: Partial<OpenFluxTransport>) => {
    setChecks((previous) => {
      const updated = { ...previous };
      delete updated[value[index].type];
      return updated;
    });
    onChange(value.map((entry, i) => (i === index ? { ...entry, ...next } : entry)));
  };
  const testTransport = async (entry: OpenFluxTransport) => {
    const type = entry.type;
    if (type === 'direct') {
      setChecks((previous) => ({
        ...previous,
        [type]: { ok: false, detail: t('pages.inbounds.form.openfluxProbeDirect') },
      }));
      return;
    }
    if (!openFluxChannelValid(entry, client)) {
      setChecks((previous) => ({
        ...previous,
        [type]: { ok: false, detail: t('pages.inbounds.form.openfluxProbeInvalid') },
      }));
      return;
    }
    if (nodeId > 0) {
      setChecks((previous) => ({
        ...previous,
        [type]: { ok: false, detail: t('pages.inbounds.form.openfluxProbeRemote') },
      }));
      return;
    }
    setChecking(type);
    try {
      // Do not transmit a URL, MAX token or account ID to the probe.
      // The server checks TLS to a fixed, trusted provider endpoint only.
      const response = await HttpUtil.post<{ host: string; reachable: boolean; scope: string }>(
        '/panel/api/server/openflux/check-transport',
        { type },
        { timeout: 12000, silent: true },
      );
      setChecks((previous) => ({
        ...previous,
        [type]:
          response.success && response.obj?.reachable
            ? {
                ok: true,
                detail: t('pages.inbounds.form.openfluxProbeReachable', {
                  host: response.obj.host,
                }),
              }
            : {
                ok: false,
                detail: response.msg || t('pages.inbounds.form.openfluxProbeFailed'),
              },
      }));
    } catch (error) {
      setChecks((previous) => ({
        ...previous,
        [type]: {
          ok: false,
          detail:
            error instanceof Error ? error.message : t('pages.inbounds.form.openfluxProbeFailed'),
        },
      }));
    } finally {
      setChecking(null);
    }
  };
  const begin = () => {
    const type = available.includes('vyandex')
      ? 'vyandex'
      : (available.find((entry) => entry !== 'direct') ?? available[0]);
    if (type)
      setDraft({ type, url: '', priority: value.length === 0 || type === 'direct' ? 100 : 50 });
  };
  return (
    <section className="openflux-channels">
      <div className="openflux-section-heading">
        <div>
          <Typography.Text strong>{t('pages.inbounds.form.openfluxChannelsTitle')}</Typography.Text>
          <Typography.Paragraph type="secondary">
            {t('pages.inbounds.form.openfluxChannelsHelp')}
          </Typography.Paragraph>
          <Typography.Paragraph type="secondary">
            {t('pages.inbounds.form.openfluxProbeScope')}
          </Typography.Paragraph>
        </div>
        <Button
          icon={<PlusOutlined aria-hidden />}
          aria-label={t('pages.inbounds.form.openfluxAdd')}
          disabled={!available.length}
          onClick={begin}
        >
          {t('pages.inbounds.form.openfluxAdd')}
        </Button>
      </div>
      {value.map((entry, index) => (
        <Card
          key={entry.type}
          size="small"
          className="openflux-channel-card"
          title={
            <Space>
              {entry.type === 'direct' ? (
                <ApiOutlined />
              ) : ['oneme', 'telemost'].includes(entry.type) ? (
                <VideoCameraOutlined />
              ) : (
                <FileTextOutlined />
              )}
              <span>{openFluxTransportLabel(entry.type)}</span>
            </Space>
          }
          extra={
            <Button
              type="text"
              danger
              icon={<DeleteOutlined />}
              disabled={value.length === 1}
              aria-label={`${t('remove')} ${openFluxTransportLabel(entry.type)}`}
              onClick={() => onChange(value.filter((_, i) => i !== index))}
            />
          }
        >
          <Typography.Paragraph type="secondary" className="openflux-channel-description">
            {t(`pages.inbounds.form.openfluxChannelDescriptions.${entry.type}`)}
          </Typography.Paragraph>
          <Collapse
            ghost
            size="small"
            items={[
              {
                key: 'setup',
                label: t('pages.inbounds.form.openfluxGuideTitle'),
                children: (
                  <Typography.Paragraph type="secondary" className="openflux-channel-note">
                    {t(`pages.inbounds.form.openfluxSetupGuide.${entry.type}`)}
                  </Typography.Paragraph>
                ),
              },
            ]}
          />
          <ChannelInputs value={entry} change={(next) => patch(index, next)} client={client} />
          <Space orientation="vertical" style={{ width: '100%' }}>
            <Button
              size="small"
              loading={checking === entry.type}
              onClick={() => void testTransport(entry)}
            >
              {t('pages.inbounds.form.openfluxProbe')}
            </Button>
            {checks[entry.type] && (
              <Alert
                type={checks[entry.type].ok ? 'success' : 'warning'}
                showIcon
                description={checks[entry.type].detail}
              />
            )}
          </Space>
          <Collapse
            ghost
            size="small"
            items={[
              {
                key: 'priority',
                label: (
                  <Space>
                    {t('pages.inbounds.form.openfluxChannelOptions')}
                    <Tag>{entry.priority ?? 50}</Tag>
                  </Space>
                ),
                children: (
                  <Form.Item
                    label={t('pages.inbounds.form.openfluxPriority')}
                    tooltip={t('pages.inbounds.form.openfluxPriorityHint')}
                  >
                    <InputNumber
                      min={0}
                      max={1000}
                      precision={0}
                      value={entry.priority ?? 50}
                      onChange={onNumber((priority) => patch(index, { priority }))}
                    />
                  </Form.Item>
                ),
              },
            ]}
          />
        </Card>
      ))}
      <Modal
        open={draft !== null}
        title={t('pages.inbounds.form.openfluxAdd')}
        onCancel={() => setDraft(null)}
        destroyOnHidden
        okText={t('add')}
        okButtonProps={{
          disabled: !draft || used.has(draft.type) || !openFluxChannelValid(draft, client),
        }}
        onOk={() => {
          if (draft && !used.has(draft.type) && openFluxChannelValid(draft, client)) {
            onChange([...value, draft]);
            setChecks((previous) => {
              const updated = { ...previous };
              delete updated[draft.type];
              return updated;
            });
            setDraft(null);
          }
        }}
      >
        {draft && (
          <Form layout="vertical" component="div">
            <Form.Item
              label={t('pages.inbounds.form.openfluxTransport')}
              tooltip={t('pages.inbounds.form.openfluxTransportHint')}
            >
              <Select
                aria-label={t('pages.inbounds.form.openfluxTransport')}
                showSearch={{ optionFilterProp: 'label' }}
                value={draft.type}
                options={available.map((type) => ({
                  value: type,
                  label: openFluxTransportLabel(type),
                }))}
                onChange={(type: OpenFluxTransportType) =>
                  setDraft({ type, url: '', priority: type === 'direct' ? 100 : 50 })
                }
              />
            </Form.Item>
            <Typography.Paragraph type="secondary">
              {t(`pages.inbounds.form.openfluxChannelDescriptions.${draft.type}`)}
            </Typography.Paragraph>
            <Typography.Paragraph type="secondary" className="openflux-channel-note">
              {t(`pages.inbounds.form.openfluxSetupGuide.${draft.type}`)}
            </Typography.Paragraph>
            <ChannelInputs
              value={draft}
              client={client}
              change={(next) => {
                const inferred = next.url ? suggestOpenFluxTransport(next.url) : undefined;
                setDraft({
                  ...draft,
                  ...next,
                  ...(inferred && available.includes(inferred) ? { type: inferred } : {}),
                });
              }}
            />
          </Form>
        )}
      </Modal>
    </section>
  );
}
