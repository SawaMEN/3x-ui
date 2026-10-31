import { useRefreshRequest } from '@/hooks/useRefreshRequest';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Checkbox, Form, Input, Modal, Select, Space, Tag, Typography } from 'antd';
import { DownloadOutlined, SyncOutlined } from '@ant-design/icons';

import { HttpUtil, FileManager } from '@/utils';
import { activateOnKey } from '@/utils/a11y';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import './LogModal.css';

interface LogModalProps {
  open: boolean;
  onClose: () => void;
}
interface DiagnosticEntry {
  source: string;
  level: string;
  time?: string;
  raw: string;
}
interface DiagnosticResponse {
  entries: DiagnosticEntry[];
  warnings: string[];
}
const EMPTY_DIAGNOSTICS: DiagnosticResponse = { entries: [], warnings: [] };
const AUTO_UPDATE_INTERVAL = 5000;

function levelColor(level: string): string {
  switch (level) {
    case 'FATAL':
    case 'ERROR': return 'red';
    case 'WARNING': return 'orange';
    case 'NOTICE': return 'cyan';
    case 'INFO': return 'green';
    case 'DEBUG': return 'blue';
    default: return 'default';
  }
}

export default function LogModal({ open, onClose }: LogModalProps) {
  const { t, i18n } = useTranslation();
  const { isMobile } = useMediaQuery();
  const ru = i18n.language.startsWith('ru');
  const [rows, setRows] = useState('200');
  const [level, setLevel] = useState('debug');
  const [source, setSource] = useState('all');
  const [query, setQuery] = useState('');
  const [autoUpdate, setAutoUpdate] = useState(false);
  const load = useCallback(
    async (signal: AbortSignal) => {
      const msg = await HttpUtil.post<DiagnosticResponse>(
        `/panel/api/server/logs/${rows}`,
        { format: 'structured', source, level, query },
        { signal },
      );
      return msg?.success && msg.obj && Array.isArray(msg.obj.entries)
        ? {
            entries: msg.obj.entries.filter(
              (entry) => entry && typeof entry.raw === 'string' && typeof entry.source === 'string',
            ),
            warnings: Array.isArray(msg.obj.warnings) ? msg.obj.warnings : [],
          }
        : undefined;
    },
    [rows, source, level, query],
  );
  const { data, loading, refresh } = useRefreshRequest(
    open, load, EMPTY_DIAGNOSTICS, autoUpdate ? AUTO_UPDATE_INTERVAL : 0,
  );
  const logs = data.entries ?? [];
  const sources = [
    { value: 'all', label: ru ? 'Все источники' : 'All sources' },
    { value: 'panel', label: ru ? 'Панель' : 'Panel' },
    { value: 'core', label: ru ? 'Ядро / ошибки' : 'Core / errors' },
    { value: 'access', label: ru ? 'Подключения Xray' : 'Xray access' },
    { value: 'journal', label: ru ? 'Службы systemd' : 'System services' },
    { value: 'protocol', label: ru ? 'Протокольные серверы' : 'Protocol servers' },
    { value: 'naive', label: 'NaiveProxy' },
    { value: 'ip-limit', label: ru ? 'Лимиты IP / блокировки' : 'IP limits / bans' },
    { value: 'update', label: ru ? 'Обновления / установка' : 'Update / install' },
  ];

  function download() {
    FileManager.downloadTextFile(
      logs.map((entry) => `[${entry.source}] ${entry.raw}`).join('\n'),
      'x-ui-diagnostics.log',
    );
  }

  return (
    <Modal
      open={open}
      footer={null}
      width={isMobile ? '100vw' : 1050}
      style={isMobile ? { top: 0, paddingBottom: 0, maxWidth: '100vw' } : undefined}
      className={isMobile ? 'logmodal-mobile' : undefined}
      onCancel={onClose}
      title={
        <>
          {t('pages.index.logs')}
          <SyncOutlined
            spin={loading}
            className="reload-icon"
            role="button"
            tabIndex={0}
            aria-label={t('refresh')}
            onClick={refresh}
            onKeyDown={activateOnKey(refresh)}
          />
        </>
      }
    >
      <Form layout="inline" className="log-toolbar">
        <Form.Item>
          <Select
            aria-label={ru ? 'Источник логов' : 'Log source'}
            value={source}
            onChange={setSource}
            size="small"
            style={{ minWidth: isMobile ? 155 : 175 }}
            options={sources}
          />
        </Form.Item>
        <Form.Item>
          <Space.Compact>
            <Select
              aria-label={t('pages.index.logLines')}
              value={rows}
              size="small"
              style={{ width: 90 }}
              onChange={setRows}
              options={['50', '100', '200', '500', '1000', '2000'].map((value) => ({ value, label: value }))}
            />
            <Select
              aria-label={t('pages.xray.logLevel')}
              value={level}
              size="small"
              style={{ minWidth: 110 }}
              onChange={setLevel}
              options={[
                { value: 'debug', label: t('pages.index.logLevelDebug') },
                { value: 'info', label: t('pages.index.logLevelInfo') },
                { value: 'notice', label: t('pages.index.logLevelNotice') },
                { value: 'warning', label: t('pages.index.logLevelWarning') },
                { value: 'error', label: t('pages.index.logLevelError') },
              ]}
            />
          </Space.Compact>
        </Form.Item>
        <Form.Item>
          <Input
            size="small"
            allowClear
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={ru ? 'Поиск по полному тексту...' : 'Search full log text...'}
            aria-label={ru ? 'Поиск по логам' : 'Search logs'}
            style={{ width: isMobile ? 210 : 235 }}
          />
        </Form.Item>
        <Form.Item>
          <Checkbox checked={autoUpdate} onChange={(event) => setAutoUpdate(event.target.checked)}>
            {t('pages.index.autoUpdate')}
          </Checkbox>
        </Form.Item>
        <Form.Item className="download-item">
          <Button
            onClick={download}
            icon={<DownloadOutlined aria-hidden />}
            aria-label={t('download')}
            title={ru ? 'Скачать все показанные записи' : 'Download all shown entries'}
          />
        </Form.Item>
      </Form>
      <Typography.Text type="secondary" className="diagnostic-summary">
        {ru
          ? `Найдено: ${logs.length}. Доступны только записи, сохранённые настройками журналирования сервера.`
          : `Found: ${logs.length}. Availability depends on the server's logging and retention settings.`}
      </Typography.Text>
      {(data.warnings?.length ?? 0) > 0 && (
        <Alert
          className="diagnostic-warnings"
          type="warning"
          showIcon
          message={ru ? 'Недоступные источники' : 'Unavailable sources'}
          description={<div>{data.warnings.map((warning, index) => <div key={index}>{warning}</div>)}</div>}
        />
      )}
      <div className={`log-container ${isMobile ? 'log-container-mobile' : ''}`} role="log" aria-live="off">
        {logs.length === 0 ? (
          <div className="log-empty">
            {ru ? 'Нет записей по фильтрам. Проверьте источник и уровень журналирования.' : 'No matching records. Check the source and log level.'}
          </div>
        ) : (
          logs.map((entry, idx) => (
            <div key={idx} className={isMobile ? 'log-card' : 'log-line diagnostic-line'}>
              <div className="diagnostic-meta">
                {entry.time && <span className="log-stamp" title={entry.time}>{new Date(entry.time).toLocaleString()}</span>}
                <Tag className="diagnostic-source">{entry.source}</Tag>
                <Tag color={levelColor(entry.level)}>{entry.level}</Tag>
              </div>
              <div className="diagnostic-raw">{entry.raw}</div>
            </div>
          ))
        )}
      </div>
    </Modal>
  );
}
