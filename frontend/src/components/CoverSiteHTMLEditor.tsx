import { useState } from 'react';
import { Alert, Button, Card, Input, Space, Typography } from 'antd';
import { useTranslation } from 'react-i18next';

export const COVER_SITE_HTML_MAX_BYTES = 256 * 1024;

// A complete, responsive example that can be edited before publishing.
export const starterCoverSiteHTML = `<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Мой сайт</title>
  <style>
    body { margin: 0; min-height: 100vh; display: grid; place-items: center;
      background: #f4f6fa; color: #202637; font: 16px/1.6 system-ui, sans-serif; }
    main { width: min(580px, 85%); padding: 48px 28px; text-align: center;
      border-radius: 22px; background: white; box-shadow: 0 18px 60px #1b244016; }
    h1 { margin: 0 0 16px; font-size: clamp(26px, 5vw, 40px); }
    p { margin: 0; color: #667085; }
  </style>
</head>
<body>
  <main>
    <h1>Добро пожаловать!</h1>
    <p>Здесь будет ваша страница. Измените этот текст и оформление.</p>
  </main>
</body>
</html>`;

export function coverSiteHTMLValidation(html: string): string | null {
  if (!html.trim() || !/<html(?:\s|>)/i.test(html) || html.includes('\0')) {
    return 'document';
  }
  if (new TextEncoder().encode(html).length > COVER_SITE_HTML_MAX_BYTES) {
    return 'size';
  }
  return null;
}

interface Props {
  value?: string;
  onChange?: (value: string) => void;
  disabled?: boolean;
}

export default function CoverSiteHTMLEditor({ value = '', onChange = () => {}, disabled }: Props) {
  const { i18n } = useTranslation();
  const ru = i18n.language.startsWith('ru');
  const [preview, setPreview] = useState(false);
  const error = coverSiteHTMLValidation(value);
  const bytes = new TextEncoder().encode(value).length;
  return (
    <Space orientation="vertical" size="small" style={{ width: '100%' }}>
      <Typography.Text strong>{ru ? 'Код вашей страницы (HTML)' : 'Your page (HTML)'}</Typography.Text>
      <Typography.Text type="secondary">
        {ru
          ? 'Вставьте готовый HTML целиком или измените пример. Для собственного дизайна используйте встроенный CSS. Файлы и адреса сайта настраиваются автоматически.'
          : 'Paste a complete HTML page or edit the example. You can embed CSS for styling. Hosting and domain settings are handled automatically.'}
      </Typography.Text>
      <Input.TextArea
        value={value}
        onChange={(event) => onChange(event.target.value)}
        autoSize={{ minRows: 10, maxRows: 18 }}
        spellCheck={false}
        disabled={disabled}
        style={{ fontFamily: 'ui-monospace, SFMono-Regular, Consolas, monospace', fontSize: 12 }}
        aria-label={ru ? 'HTML сайта' : 'Website HTML'}
      />
      <Space wrap style={{ justifyContent: 'space-between', width: '100%' }}>
        <Typography.Text type={error === 'size' ? 'danger' : 'secondary'}>
          {new Intl.NumberFormat(ru ? 'ru-RU' : 'en-US').format(bytes)} / 262 144 {ru ? 'байт' : 'bytes'}
        </Typography.Text>
        <Button size="small" onClick={() => setPreview((current) => !current)}>
          {preview ? (ru ? 'Скрыть предпросмотр' : 'Hide preview') : (ru ? 'Предпросмотр' : 'Preview')}
        </Button>
      </Space>
      {error && (
        <Alert
          type="warning"
          showIcon
          title={error === 'size'
            ? (ru ? 'Максимальный размер — 256 КБ' : 'Maximum size is 256 KiB')
            : (ru ? 'Нужен полный HTML-документ с тегом <html>' : 'A full HTML document with an <html> tag is required')}
        />
      )}
      {preview && (
        <Card size="small" title={ru ? 'Предпросмотр страницы' : 'Page preview'}>
          <iframe
            title={ru ? 'Безопасный предпросмотр сайта' : 'Sandboxed website preview'}
            sandbox=""
            referrerPolicy="no-referrer"
            srcDoc={value}
            style={{ border: '1px solid var(--ant-color-border, #ddd)', borderRadius: 8, width: '100%', height: 310, background: '#fff' }}
          />
          <Typography.Text type="secondary">
            {ru ? 'В предпросмотре сценарии отключены.' : 'Scripts are disabled in this preview.'}
          </Typography.Text>
        </Card>
      )}
    </Space>
  );
}
