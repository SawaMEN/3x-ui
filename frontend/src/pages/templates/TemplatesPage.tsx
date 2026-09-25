import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Card,
  Col,
  ConfigProvider,
  Empty,
  Input,
  Layout,
  Modal,
  Row,
  Select,
  Space,
  Spin,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  CodeOutlined,
  CopyOutlined,
  DeleteOutlined,
  DownloadOutlined,
  FileProtectOutlined,
  PlusOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons';

import AppSidebar from '@/layouts/AppSidebar';
import { HttpUtil, ClipboardManager } from '@/utils';
import { useTheme } from '@/hooks/useTheme';
import './TemplatesPage.css';

type Template = {
  id: number;
  kind: 'inbound' | 'xray_config';
  title: string;
  description: string;
  tags: string[];
  sizeBytes: number;
  createdAt: number;
  updatedAt: number;
  summary?: Record<string, unknown>;
};

type TemplateDetail = Template & {
  content: unknown;
};

type TemplateListResponse = {
  items?: Template[];
  total?: number;
};

type TemplateDetailResponse = {
  id?: number;
  kind?: Template['kind'];
  title?: string;
  description?: string;
  tags?: string[];
  sizeBytes?: number;
  createdAt?: number;
  updatedAt?: number;
  summary?: Record<string, unknown>;
  content?: unknown;
};

type SaveResponse = {
  template?: Template;
  warnings?: string[];
};

type SanitizeResponse = {
  kind?: Template['kind'];
  content?: unknown;
  warnings?: string[];
  sizeBytes?: number;
};

const emptyDraft = {
  kind: 'inbound' as Template['kind'],
  title: '',
  description: '',
  tags: '',
  content: '{\n  "protocol": "vless",\n  "settings": {\n    "clients": []\n  }\n}',
};

function pretty(value: unknown): string {
  if (typeof value === 'string') return value;
  return JSON.stringify(value ?? {}, null, 2);
}

function formatBytes(size: number): string {
  if (!Number.isFinite(size) || size <= 0) return '0 B';
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

function formatDate(unix: number): string {
  if (!unix) return '-';
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(unix * 1000));
}

export default function TemplatesPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { antdThemeConfig, isDark, isUltra } = useTheme();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [items, setItems] = useState<Template[]>([]);
  const [loading, setLoading] = useState(true);
  const [query, setQuery] = useState('');
  const [kind, setKind] = useState<'' | Template['kind']>('');
  const [editorOpen, setEditorOpen] = useState(false);
  const [detailOpen, setDetailOpen] = useState(false);
  const [draft, setDraft] = useState(emptyDraft);
  const [preview, setPreview] = useState<SanitizeResponse | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [saveLoading, setSaveLoading] = useState(false);
  const [detail, setDetail] = useState<TemplateDetail | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const msg = await HttpUtil.get<TemplateListResponse>(
        `/panel/api/templates/list?kind=${encodeURIComponent(kind)}&q=${encodeURIComponent(query)}&limit=50`,
        undefined,
        { silent: true },
      );
      if (!msg?.success) throw new Error(msg?.msg || t('pages.templates.loadFailed'));
      setItems(msg?.obj?.items || []);
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('pages.templates.loadFailed'));
    } finally {
      setLoading(false);
    }
  }, [kind, messageApi, query, t]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 180);
    return () => window.clearTimeout(timer);
  }, [load]);

  const openEditor = useCallback(() => {
    setDraft(emptyDraft);
    setPreview(null);
    setEditorOpen(true);
  }, []);

  const sanitize = useCallback(async () => {
    setPreviewLoading(true);
    try {
      let content: unknown;
      try {
        content = JSON.parse(draft.content);
      } catch {
        throw new Error(t('pages.templates.invalidJson'));
      }
      const msg = await HttpUtil.post<SanitizeResponse>(
        '/panel/api/templates/sanitize',
        { kind: draft.kind, content },
        { headers: { 'Content-Type': 'application/json' } },
      );
      if (!msg?.success || !msg.obj) throw new Error(msg?.msg || t('pages.templates.sanitizeFailed'));
      setPreview(msg.obj);
      return msg.obj;
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('pages.templates.sanitizeFailed'));
      return null;
    } finally {
      setPreviewLoading(false);
    }
  }, [draft.content, draft.kind, messageApi, t]);

  const save = useCallback(async () => {
    if (!draft.title.trim()) {
      messageApi.warning(t('pages.templates.titleRequired'));
      return;
    }
    setSaveLoading(true);
    try {
      let content: unknown;
      try {
        content = JSON.parse(draft.content);
      } catch {
        throw new Error(t('pages.templates.invalidJson'));
      }
      const msg = await HttpUtil.post<SaveResponse>(
        '/panel/api/templates/save',
        {
          kind: draft.kind,
          title: draft.title,
          description: draft.description,
          tags: draft.tags
            .split(',')
            .map((tag) => tag.trim())
            .filter(Boolean),
          content,
        },
        { headers: { 'Content-Type': 'application/json' } },
      );
      if (!msg?.success || !msg.obj?.template) {
        throw new Error(msg?.msg || t('pages.templates.saveFailed'));
      }
      if ((msg.obj.warnings || []).length > 0) {
        messageApi.warning(t('pages.templates.savedWithWarnings'));
      } else {
        messageApi.success(t('pages.templates.saved'));
      }
      setEditorOpen(false);
      await load();
    } catch (error) {
      messageApi.error(error instanceof Error ? error.message : t('pages.templates.saveFailed'));
    } finally {
      setSaveLoading(false);
    }
  }, [draft, load, messageApi, t]);

  const openDetail = useCallback(
    async (id: number) => {
      try {
        const msg = await HttpUtil.get<TemplateDetailResponse>(`/panel/api/templates/get/${id}`);
        if (!msg?.success || !msg.obj) throw new Error(msg?.msg || t('pages.templates.loadFailed'));
        setDetail(msg.obj as TemplateDetail);
        setDetailOpen(true);
      } catch (error) {
        messageApi.error(error instanceof Error ? error.message : t('pages.templates.loadFailed'));
      }
    },
    [messageApi, t],
  );

  const remove = useCallback(
    (item: Template) => {
      Modal.confirm({
        title: t('pages.templates.deleteTitle'),
        content: item.title,
        okType: 'danger',
        okText: t('delete'),
        cancelText: t('cancel'),
        onOk: async () => {
          const msg = await HttpUtil.post(`/panel/api/templates/del/${item.id}`);
          if (!msg?.success) {
            messageApi.error(msg?.msg || t('pages.templates.deleteFailed'));
            return;
          }
          messageApi.success(t('pages.templates.deleted'));
          await load();
        },
      });
    },
    [load, messageApi, t],
  );

  const exportDetail = useCallback(async () => {
    if (!detail) return;
    const safeName =
      detail.title.replace(/[^a-z0-9_-]+/gi, '-').replace(/^-+|-+$/g, '') || 'template';
    const blob = new Blob([pretty(detail.content)], { type: 'application/json;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `${safeName}.json`;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    URL.revokeObjectURL(url);
  }, [detail]);

  const applyInbound = useCallback(async () => {
    if (!detail || detail.kind !== 'inbound') return;
    const content =
      detail.content && typeof detail.content === 'object'
        ? { ...(detail.content as Record<string, unknown>), remark: detail.title }
        : detail.content;
    const msg = await HttpUtil.post('/panel/api/inbounds/import', {
      data: JSON.stringify(content),
    });
    if (!msg?.success) return;
    setDetailOpen(false);
    messageApi.success(t('pages.templates.applied'));
    navigate('/inbounds');
  }, [detail, messageApi, navigate, t]);

  const copyDetail = useCallback(async () => {
    if (!detail) return;
    const ok = await ClipboardManager.copyText(pretty(detail.content));
    if (ok) messageApi.success(t('pages.templates.copied'));
  }, [detail, messageApi, t]);

  const pageClass = ['templates-page', isDark ? 'is-dark' : '', isUltra ? 'is-ultra' : '']
    .filter(Boolean)
    .join(' ');

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <div className="templates-header">
              <div>
                <Typography.Title level={2} style={{ marginBottom: 4 }}>
                  {t('pages.templates.title')}
                </Typography.Title>
                <Typography.Paragraph type="secondary" style={{ margin: 0 }}>
                  {t('pages.templates.subtitle')}
                </Typography.Paragraph>
              </div>
              <Space wrap>
                <Button icon={<ReloadOutlined />} onClick={() => void load()} loading={loading}>
                  {t('refresh')}
                </Button>
                <Button type="primary" icon={<PlusOutlined />} onClick={openEditor}>
                  {t('pages.templates.add')}
                </Button>
              </Space>
            </div>

            <Card className="templates-toolbar" size="small">
              <Space wrap style={{ width: '100%' }}>
                <Input.Search
                  allowClear
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder={t('pages.templates.searchPlaceholder')}
                  style={{ minWidth: 260, flex: 1 }}
                />
                <Select
                  value={kind}
                  onChange={(value) => setKind(value as '' | Template['kind'])}
                  style={{ minWidth: 180 }}
                  options={[
                    { value: '', label: t('pages.templates.allKinds') },
                    { value: 'inbound', label: t('pages.templates.inbound') },
                    { value: 'xray_config', label: t('pages.templates.xrayConfig') },
                  ]}
                />
              </Space>
            </Card>

            <Spin spinning={loading}>
              {items.length === 0 ? (
                <Card className="templates-empty">
                  <Empty
                    image={<SafetyCertificateOutlined style={{ fontSize: 48 }} />}
                    description={t('pages.templates.empty')}
                  />
                </Card>
              ) : (
                <Row gutter={[14, 14]}>
                  {items.map((item) => (
                    <Col key={item.id} xs={24} sm={12} lg={8} xl={6}>
                      <Card
                        hoverable
                        className="template-card"
                        actions={[
                          <Button
                            key="open"
                            type="text"
                            icon={<CodeOutlined />}
                            onClick={() => void openDetail(item.id)}
                          />,
                          <Button
                            key="delete"
                            type="text"
                            danger
                            icon={<DeleteOutlined />}
                            onClick={() => remove(item)}
                          />,
                        ]}
                      >
                        <div className="template-card-kind">
                          <FileProtectOutlined />
                          <span>
                            {item.kind === 'inbound'
                              ? t('pages.templates.inbound')
                              : t('pages.templates.xrayConfig')}
                          </span>
                        </div>
                        <Typography.Title level={4} ellipsis={{ rows: 2 }} className="template-card-title">
                          {item.title}
                        </Typography.Title>
                        <Typography.Paragraph
                          type="secondary"
                          ellipsis={{ rows: 3 }}
                          className="template-card-description"
                        >
                          {item.description || t('pages.templates.noDescription')}
                        </Typography.Paragraph>
                        <div className="template-card-tags">
                          {item.tags.map((tag) => (
                            <Tag key={tag}>{tag}</Tag>
                          ))}
                        </div>
                        <div className="template-card-meta">
                          <span>{formatBytes(item.sizeBytes)}</span>
                          <span>{formatDate(item.updatedAt)}</span>
                        </div>
                        {item.summary && Object.keys(item.summary).length > 0 && (
                          <div className="template-card-summary">
                            {Object.entries(item.summary).map(([key, value]) => (
                              <Tag color="blue" key={key}>
                                {key}: {String(value)}
                              </Tag>
                            ))}
                          </div>
                        )}
                      </Card>
                    </Col>
                  ))}
                </Row>
              )}
            </Spin>
          </Layout.Content>
        </Layout>
      </Layout>

      <Modal
        open={editorOpen}
        title={t('pages.templates.editorTitle')}
        width={920}
        destroyOnHidden
        onCancel={() => setEditorOpen(false)}
        footer={[
          <Button key="cancel" onClick={() => setEditorOpen(false)}>
            {t('cancel')}
          </Button>,
          <Button
            key="preview"
            icon={<SafetyCertificateOutlined />}
            loading={previewLoading}
            onClick={() => void sanitize()}
          >
            {t('pages.templates.previewSanitize')}
          </Button>,
          <Button key="save" type="primary" loading={saveLoading} onClick={() => void save()}>
            {t('save')}
          </Button>,
        ]}
      >
        <Space direction="vertical" size={12} style={{ width: '100%' }}>
          <Row gutter={12}>
            <Col xs={24} md={12}>
              <Typography.Text strong>{t('pages.templates.kind')}</Typography.Text>
              <Select
                value={draft.kind}
                onChange={(value) => setDraft((v) => ({ ...v, kind: value }))}
                style={{ width: '100%', marginTop: 4 }}
                options={[
                  { value: 'inbound', label: t('pages.templates.inbound') },
                  { value: 'xray_config', label: t('pages.templates.xrayConfig') },
                ]}
              />
            </Col>
            <Col xs={24} md={12}>
              <Typography.Text strong>{t('pages.templates.name')}</Typography.Text>
              <Input
                value={draft.title}
                onChange={(e) => setDraft((v) => ({ ...v, title: e.target.value }))}
                placeholder={t('pages.templates.namePlaceholder')}
                style={{ marginTop: 4 }}
              />
            </Col>
          </Row>
          <Input
            value={draft.description}
            onChange={(e) => setDraft((v) => ({ ...v, description: e.target.value }))}
            placeholder={t('pages.templates.descriptionPlaceholder')}
          />
          <Input
            value={draft.tags}
            onChange={(e) => setDraft((v) => ({ ...v, tags: e.target.value }))}
            placeholder={t('pages.templates.tagsPlaceholder')}
          />
          <Input.TextArea
            value={draft.content}
            onChange={(e) => setDraft((v) => ({ ...v, content: e.target.value }))}
            autoSize={{ minRows: 18, maxRows: 30 }}
            spellCheck={false}
            style={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}
          />
          {preview && (
            <Alert
              type={preview.warnings?.length ? 'warning' : 'success'}
              showIcon
              icon={<SafetyCertificateOutlined />}
              message={t('pages.templates.sanitizeResult', { size: formatBytes(preview.sizeBytes || 0) })}
              description={
                preview.warnings?.length ? (
                  <ul style={{ marginBottom: 0, paddingInlineStart: 18 }}>
                    {preview.warnings.slice(0, 12).map((warning) => (
                      <li key={warning}>{warning}</li>
                    ))}
                  </ul>
                ) : (
                  t('pages.templates.noWarnings')
                )
              }
            />
          )}
        </Space>
      </Modal>

      <Modal
        open={detailOpen}
        title={detail?.title}
        width={980}
        footer={
          <Space>
            {detail?.kind === 'inbound' && (
              <Button type="primary" icon={<PlusOutlined />} onClick={() => void applyInbound()}>
                {t('pages.templates.applyInbound')}
              </Button>
            )}
            <Button icon={<CopyOutlined />} onClick={() => void copyDetail()}>
              {t('pages.templates.copy')}
            </Button>
            <Button icon={<DownloadOutlined />} onClick={exportDetail}>
              {t('pages.templates.export')}
            </Button>
          </Space>
        }
        onCancel={() => setDetailOpen(false)}
      >
        {detail && (
          <>
            <Space wrap style={{ marginBottom: 12 }}>
              <Tag color="blue">
                {detail.kind === 'inbound'
                  ? t('pages.templates.inbound')
                  : t('pages.templates.xrayConfig')}
              </Tag>
              {detail.tags.map((tag) => (
                <Tag key={tag}>{tag}</Tag>
              ))}
            </Space>
            {detail.description && (
              <Typography.Paragraph type="secondary">{detail.description}</Typography.Paragraph>
            )}
            <pre className="template-json-view">{pretty(detail.content)}</pre>
          </>
        )}
      </Modal>
    </ConfigProvider>
  );
}
