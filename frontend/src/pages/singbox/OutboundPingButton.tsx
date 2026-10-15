import { useState } from 'react';
import { Button, Space, Tag, Typography } from 'antd';
import { HttpUtil } from '@/utils';

export default function OutboundPingButton({
  tag,
  disabled = false,
}: {
  tag: string;
  disabled?: boolean;
}) {
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<{ delay?: number; error?: string } | null>(null);
  async function measure() {
    setBusy(true);
    setResult(null);
    try {
      const reply = await HttpUtil.post<{ delay?: number }>(
        '/panel/api/server/singbox/outbound/check',
        { tag, timeout: 10000 },
        { silent: true },
      );
      if (!reply.success) throw new Error(reply.msg || 'Подключение недоступно');
      const delay = reply.obj?.delay;
      if (typeof delay !== 'number' || !Number.isFinite(delay) || delay < 0)
        throw new Error('Ядро не вернуло результат проверки');
      setResult({ delay });
    } catch (error) {
      setResult({ error: error instanceof Error ? error.message : String(error) });
    } finally {
      setBusy(false);
    }
  }
  return (
    <Space wrap>
      <span
        title={
          disabled
            ? 'Сначала сохраните подключение и дождитесь применения ядром'
            : 'Задержка HTTP-запроса через применённое подключение'
        }
      >
        <Button
          size="small"
          loading={busy}
          disabled={disabled || !tag}
          onClick={() => void measure()}
          title="Задержка HTTP-запроса через сохранённое подключение"
        >
          Пинг
        </Button>
      </span>
      {result?.delay !== undefined && <Tag color="green">{result.delay} мс</Tag>}
      {result?.error && <Typography.Text type="warning">{result.error}</Typography.Text>}
    </Space>
  );
}
