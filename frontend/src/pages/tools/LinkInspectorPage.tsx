import { useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Col,
  ConfigProvider,
  Descriptions,
  Input,
  Layout,
  Row,
  Space,
  Tag,
  Typography,
} from 'antd';

import { useTheme } from '@/hooks/useTheme';
import AppSidebar from '@/layouts/AppSidebar';
import { inspectProxyLink, type InspectedProxyLink } from '@/lib/xray/link-inspector';

const { Paragraph, Text, Title } = Typography;

function CopyValue({ value }: { value: string }) {
  if (!value) return <Text type="secondary">—</Text>;
  return <Text copyable>{value}</Text>;
}

export default function LinkInspectorPage() {
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const [input, setInput] = useState('');
  const [result, setResult] = useState<InspectedProxyLink | null>(null);
  const [error, setError] = useState('');

  const pageClass = `link-inspector-page ${isDark ? 'is-dark' : ''} ${isUltra ? 'is-ultra' : ''}`.trim();
  const paramsText = useMemo(() => (result ? JSON.stringify(result.params, null, 2) : ''), [result]);

  function inspect() {
    try {
      setResult(inspectProxyLink(input));
      setError('');
    } catch (err) {
      setResult(null);
      setError(err instanceof Error ? err.message : 'Unable to parse link');
    }
  }

  function clear() {
    setInput('');
    setResult(null);
    setError('');
  }

  const descriptionItems = result
    ? [
        { key: 'protocol', label: 'protocol', children: <Tag>{result.protocol}</Tag> },
        { key: 'name', label: 'name', children: <CopyValue value={result.name} /> },
        { key: 'host', label: 'host', children: <CopyValue value={result.host} /> },
        { key: 'port', label: 'port', children: result.port ?? '—' },
        { key: 'credential', label: 'credential', children: <CopyValue value={result.credential} /> },
        { key: 'network', label: 'network', children: <CopyValue value={result.network} /> },
        { key: 'security', label: 'security', children: <CopyValue value={result.security} /> },
        { key: 'sni', label: 'sni', children: <CopyValue value={result.sni} /> },
        { key: 'hostHeader', label: 'host header', children: <CopyValue value={result.hostHeader} /> },
        { key: 'path', label: 'path', children: <CopyValue value={result.path} /> },
        { key: 'fingerprint', label: 'fingerprint', children: <CopyValue value={result.fingerprint} /> },
        { key: 'publicKey', label: 'reality public key', children: <CopyValue value={result.publicKey} /> },
        { key: 'shortId', label: 'reality short id', children: <CopyValue value={result.shortId} /> },
      ]
    : [];

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Row gutter={[16, 16]}>
              <Col span={24}>
                <Card hoverable>
                  <Title level={3} style={{ marginTop: 0 }}>
                    Link Inspector
                  </Title>
                  <Paragraph type="secondary">
                    VLESS / VMess / Trojan / Shadowsocks / HTTP / SOCKS. Parsing is performed locally in the browser.
                  </Paragraph>
                  <Input.TextArea
                    value={input}
                    autoSize={{ minRows: 4, maxRows: 10 }}
                    placeholder="vless://..."
                    spellCheck={false}
                    onChange={(event) => setInput(event.target.value)}
                    onPressEnter={(event) => {
                      if (event.ctrlKey || event.metaKey) inspect();
                    }}
                  />
                  <Space wrap style={{ marginTop: 12 }}>
                    <Button type="primary" onClick={inspect} disabled={!input.trim()}>
                      Parse
                    </Button>
                    <Button onClick={clear} disabled={!input && !result && !error}>
                      Clear
                    </Button>
                  </Space>
                </Card>
              </Col>

              {error && (
                <Col span={24}>
                  <Alert type="error" showIcon title={error} />
                </Col>
              )}

              {result && (
                <>
                  <Col span={24}>
                    <Card title="Decoded link" hoverable>
                      <Descriptions bordered size="small" column={{ xs: 1, sm: 1, md: 2 }} items={descriptionItems} />
                    </Card>
                  </Col>

                  {result.issues.length > 0 && (
                    <Col span={24}>
                      <Alert
                        type="warning"
                        showIcon
                        title="Validation warnings"
                        description={result.issues.map((issue) => (
                          <div key={issue}>{issue}</div>
                        ))}
                      />
                    </Col>
                  )}

                  <Col span={24}>
                    <Card title="Parameters" hoverable>
                      <Paragraph
                        copyable={{ text: paramsText }}
                        style={{ marginBottom: 0, whiteSpace: 'pre-wrap', fontFamily: 'monospace' }}
                      >
                        {paramsText}
                      </Paragraph>
                    </Card>
                  </Col>
                </>
              )}
            </Row>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
