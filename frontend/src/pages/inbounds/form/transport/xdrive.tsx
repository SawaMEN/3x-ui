import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Collapse,
  Input,
  InputNumber,
  Select,
  Space,
  Switch,
  Typography,
} from 'antd';
import { useFormContext, useWatch } from 'react-hook-form';

import { JsonEditor } from '@/components/form';
import { FormField } from '@/components/form/rhf';
import { XDriveServiceSchema } from '@/schemas/protocols/stream/xdrive';
import { HttpUtil } from '@/utils';
import {
  identifyYandexStoragePreset,
  validYandexFolder,
  validYandexBucket,
  yandexStoragePresetValues,
  type YandexStoragePreset,
} from '@/lib/xray/yandex-storage';

const BASE = ['streamSettings', 'xdriveSettings'] as const;

const SERVICE_LABEL_KEYS: Partial<Record<string, string>> = {
  local: 'pages.inbounds.form.xdrive.serviceLocal',
  template: 'pages.inbounds.form.xdrive.serviceTemplate',
};
const SERVICE_NAMES: Partial<Record<string, string>> = { 'google-drive': 'Google Drive', s3: 'S3' };

// Core defaults (transport/internet/xdrive/params.go), shown as placeholders so an
// empty field keeps meaning "use the core's value".
const TUNING_FIELDS = [
  ['segmentBytes', 'segmentBytes', 524288],
  ['flushIntervalMs', 'flushInterval', 20],
  ['pollIntervalMs', 'pollInterval', 50],
  ['maxPollIntervalMs', 'maxPollInterval', 500],
  ['eagerWindowMs', 'eagerWindow', 2000],
  ['holeTimeoutMs', 'holeTimeout', 30000],
  ['sessionTtlSeconds', 'sessionTtl', 300],
  ['concurrency', 'concurrency', 8],
] as const;

const GOOGLE_DRIVE_SECRETS = ['clientId', 'clientSecret', 'refreshToken'] as const;

function TemplateEditor({
  value,
  onChange,
}: {
  value?: Record<string, unknown>;
  onChange?: (next: Record<string, unknown> | undefined) => void;
}) {
  const { t } = useTranslation();
  const [text, setText] = useState(() => (value ? JSON.stringify(value, null, 2) : ''));
  const [invalid, setInvalid] = useState(false);
  return (
    <>
      <JsonEditor
        value={text}
        minHeight="160px"
        onChange={(next) => {
          setText(next);
          if (next.trim() === '') {
            setInvalid(false);
            onChange?.(undefined);
            return;
          }
          try {
            const parsed: unknown = JSON.parse(next);
            const isObject = !!parsed && typeof parsed === 'object' && !Array.isArray(parsed);
            setInvalid(!isObject);
            if (isObject) onChange?.(parsed as Record<string, unknown>);
          } catch {
            setInvalid(true);
          }
        }}
      />
      {invalid && (
        <Typography.Text type="danger">
          {t('pages.inbounds.form.xdrive.templateInvalid')}
        </Typography.Text>
      )}
    </>
  );
}

export default function XDriveForm() {
  const { t } = useTranslation();
  const { control, setValue, getValues } = useFormContext();
  const service = useWatch({ control, name: `${BASE.join('.')}.service` }) as string | undefined;
  const template = useWatch({ control, name: `${BASE.join('.')}.template` }) as
    | Record<string, unknown>
    | undefined;
  const s3 = useWatch({ control, name: `${BASE.join('.')}.s3` }) as
    | { endpoint?: string; bucket?: string }
    | undefined;
  const folder = useWatch({ control, name: `${BASE.join('.')}.remoteFolder` }) as
    | string
    | undefined;
  const secrets = useWatch({ control, name: `${BASE.join('.')}.secrets` }) as string[] | undefined;
  const nodeId = useWatch({ control, name: 'nodeId' }) as number | undefined;
  const preset = identifyYandexStoragePreset({ service, template, s3 });
  const [checking, setChecking] = useState(false);
  const [check, setCheck] = useState<{ ok: boolean; message: string } | null>(null);

  const chooseYandex = (next: YandexStoragePreset) => {
    setCheck(null);
    if (next === 'custom') {
      setValue(
        BASE.join('.'),
        { service: 'template', remoteFolder: folder || 'xdrive', template: {}, secrets: [] },
        { shouldDirty: true, shouldValidate: true },
      );
      return;
    }
    const values = yandexStoragePresetValues(next);
    const previous = getValues(`${BASE.join('.')}.remoteFolder`) as string | undefined;
    setValue(
      BASE.join('.'),
      {
        ...values,
        remoteFolder: previous && validYandexFolder(previous) ? previous : values.remoteFolder,
      },
      { shouldDirty: true, shouldValidate: true },
    );
    setValue('streamSettings.address', undefined, { shouldDirty: true });
    setValue('streamSettings.port', undefined, { shouldDirty: true });
  };

  const testYandexStorage = async () => {
    if (preset !== 'webdav' && preset !== 'disk-api') return;
    if (nodeId && nodeId > 0) {
      setCheck({ ok: false, message: t('pages.inbounds.form.xdrive.yandexProbeRemote') });
      return;
    }
    const webdav = preset === 'webdav';
    if (
      !folder ||
      !validYandexFolder(folder) ||
      !secrets?.[0]?.trim() ||
      (webdav && !secrets?.[1])
    ) {
      setCheck({
        ok: false,
        message: t(
          webdav
            ? 'pages.inbounds.form.xdrive.yandexProbeIncomplete'
            : 'pages.inbounds.form.xdrive.yandexApiProbeIncomplete',
        ),
      });
      return;
    }
    setChecking(true);
    try {
      const response = await HttpUtil.post<{
        folderCreated: boolean;
        written: boolean;
        read: boolean;
        listed: boolean;
        deleted: boolean;
      }>(
        webdav ? '/panel/api/xdrive/yandex/probe' : '/panel/api/xdrive/yandex/api-probe',
        webdav
          ? { login: secrets[0], appPassword: secrets[1], folder }
          : { token: secrets[0], folder },
        { silent: true, timeout: 40000 },
      );
      setCheck({
        ok:
          response.success &&
          !!response.obj?.written &&
          !!response.obj?.read &&
          !!response.obj?.listed &&
          !!response.obj?.deleted,
        message: response.success
          ? t(
              webdav
                ? 'pages.inbounds.form.xdrive.yandexProbePassed'
                : 'pages.inbounds.form.xdrive.yandexApiProbePassed',
              { folder },
            )
          : response.msg || t('pages.inbounds.form.xdrive.yandexProbeFailed'),
      });
    } finally {
      setChecking(false);
    }
  };
  const serviceOptions = XDriveServiceSchema.options.map((value) => {
    const labelKey = SERVICE_LABEL_KEYS[value];
    return { value, label: labelKey ? t(labelKey) : (SERVICE_NAMES[value] ?? value) };
  });

  return (
    <>
      <Alert
        type="warning"
        showIcon
        description={t('pages.inbounds.form.xdrive.nativeHint')}
        style={{ marginBottom: 12 }}
      />
      <Typography.Title level={5} style={{ marginTop: 0 }}>
        {t('pages.inbounds.form.xdrive.yandexWizardTitle')}
      </Typography.Title>
      <Typography.Paragraph type="secondary">
        {t('pages.inbounds.form.xdrive.yandexWizardIntro')}
      </Typography.Paragraph>
      <Select
        aria-label={t('pages.inbounds.form.xdrive.yandexWizardTitle')}
        style={{ width: '100%', marginBottom: 12 }}
        value={preset}
        options={[
          { value: 'custom', label: t('pages.inbounds.form.xdrive.yandexCustom') },
          { value: 'webdav', label: t('pages.inbounds.form.xdrive.yandexWebdav') },
          { value: 'disk-api', label: t('pages.inbounds.form.xdrive.yandexDiskApi') },
          { value: 'object-storage', label: t('pages.inbounds.form.xdrive.yandexObjectStorage') },
        ]}
        onChange={(value: YandexStoragePreset) => chooseYandex(value)}
      />
      {preset !== 'custom' && (
        <Alert
          style={{ marginBottom: 12 }}
          type="info"
          showIcon
          description={t(`pages.inbounds.form.xdrive.yandexInfo.${preset}`)}
        />
      )}
      <FormField label={t('pages.inbounds.form.xdrive.service')} name={[...BASE, 'service']}>
        <Select options={serviceOptions} />
      </FormField>
      <FormField
        label={t('pages.inbounds.form.xdrive.remoteFolder')}
        name={[...BASE, 'remoteFolder']}
        onAfterChange={() => setCheck(null)}
        required
      >
        <Input placeholder={service === 'local' ? '/var/lib/xdrive' : 'xray-tunnel'} />
      </FormField>
      {service === 'google-drive' &&
        GOOGLE_DRIVE_SECRETS.map((key, index) => (
          <FormField
            key={key}
            label={t(`pages.inbounds.form.xdrive.${key}`)}
            name={[...BASE, 'secrets', index]}
            required
          >
            {key === 'clientId' ? <Input /> : <Input.Password />}
          </FormField>
        ))}
      {service === 's3' && (
        <>
          {preset === 'object-storage' && (
            <Typography.Paragraph type="secondary">
              {t('pages.inbounds.form.xdrive.yandexBucketHint')}{' '}
              <Typography.Link
                href="https://console.yandex.cloud/"
                target="_blank"
                rel="noopener noreferrer"
              >
                {t('pages.inbounds.form.xdrive.yandexCloudLink')}
              </Typography.Link>
            </Typography.Paragraph>
          )}
          {(['endpoint', 'region', 'bucket'] as const).map((key) => (
            <FormField
              key={key}
              name={[...BASE, 's3', key]}
              label={t(`pages.inbounds.form.xdrive.s3.${key}`)}
            >
              <Input />
            </FormField>
          ))}
          <FormField
            name={[...BASE, 's3', 'pathStyle']}
            label={t('pages.inbounds.form.xdrive.s3.pathStyle')}
            valueProp="checked"
          >
            <Switch />
          </FormField>
          {(['accessKey', 'secretKey', 'sessionToken'] as const).map((key, index) => (
            <FormField
              key={key}
              name={[...BASE, 'secrets', index]}
              label={t(`pages.inbounds.form.xdrive.s3.${key}`)}
            >
              <Input.Password />
            </FormField>
          ))}
        </>
      )}
      {preset === 'webdav' && (
        <>
          <FormField
            label={t('pages.inbounds.form.xdrive.yandexLogin')}
            name={[...BASE, 'secrets', 0]}
            onAfterChange={() => setCheck(null)}
            required
          >
            <Input autoComplete="username" placeholder="your-login" />
          </FormField>
          <FormField
            label={t('pages.inbounds.form.xdrive.yandexAppPassword')}
            name={[...BASE, 'secrets', 1]}
            onAfterChange={() => setCheck(null)}
            required
          >
            <Input.Password autoComplete="off" />
          </FormField>
          <Space orientation="vertical" style={{ marginBottom: 12, width: '100%' }}>
            <Typography.Link
              href="https://id.yandex.ru/security/app-passwords"
              target="_blank"
              rel="noopener noreferrer"
            >
              {t('pages.inbounds.form.xdrive.yandexPasswordLink')}
            </Typography.Link>
            <Button loading={checking} onClick={() => void testYandexStorage()}>
              {t('pages.inbounds.form.xdrive.yandexProbe')}
            </Button>
            {check && (
              <Alert showIcon type={check.ok ? 'success' : 'warning'} description={check.message} />
            )}
          </Space>
        </>
      )}
      {preset === 'disk-api' && (
        <>
          <FormField
            label={t('pages.inbounds.form.xdrive.yandexOauthToken')}
            name={[...BASE, 'secrets', 0]}
            onAfterChange={() => setCheck(null)}
            required
          >
            <Input.Password autoComplete="off" />
          </FormField>
          <Typography.Paragraph type="secondary">
            {t('pages.inbounds.form.xdrive.yandexOauthHint')}
          </Typography.Paragraph>
          <Typography.Link
            href="https://yandex.ru/dev/disk-api/doc/ru/concepts/quickstart"
            target="_blank"
            rel="noopener noreferrer"
          >
            {t('pages.inbounds.form.xdrive.yandexOauthLink')}
          </Typography.Link>
          <Space
            orientation="vertical"
            style={{ display: 'flex', marginTop: 12, marginBottom: 12 }}
          >
            <Button loading={checking} onClick={() => void testYandexStorage()}>
              {t('pages.inbounds.form.xdrive.yandexProbe')}
            </Button>
            {check && (
              <Alert showIcon type={check.ok ? 'success' : 'warning'} description={check.message} />
            )}
          </Space>
        </>
      )}
      {service === 'template' && preset === 'custom' && (
        <>
          <FormField
            label={t('pages.inbounds.form.xdrive.secrets')}
            tooltip={t('pages.inbounds.form.xdrive.secretsDesc')}
            name={[...BASE, 'secrets']}
          >
            <Select mode="tags" tokenSeparators={[',']} open={false} />
          </FormField>
          <FormField
            label={t('pages.inbounds.form.xdrive.template')}
            name={[...BASE, 'template']}
            required
          >
            <TemplateEditor />
          </FormField>
        </>
      )}
      {(preset === 'webdav' || preset === 'disk-api') && folder && !validYandexFolder(folder) && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 12 }}
          description={t('pages.inbounds.form.xdrive.yandexFolderInvalid')}
        />
      )}
      {preset === 'object-storage' && s3?.bucket && !validYandexBucket(s3.bucket) && (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 12 }}
          description={t('pages.inbounds.form.xdrive.yandexBucketInvalid')}
        />
      )}
      {service !== 'local' && (
        <>
          <FormField
            label={t('pages.inbounds.form.xdrive.front')}
            tooltip={t('pages.inbounds.form.xdrive.frontDesc')}
            name={['streamSettings', 'address']}
            transform={{ output: (raw) => String(raw ?? '').trim() || undefined }}
          >
            <Input placeholder="www.googleapis.com" />
          </FormField>
          <FormField
            label={t('pages.inbounds.form.xdrive.frontPort')}
            name={['streamSettings', 'port']}
          >
            <InputNumber min={1} max={65535} placeholder="443" />
          </FormField>
        </>
      )}
      <Collapse
        ghost
        style={{ marginBottom: 12 }}
        items={[
          {
            key: 'yandex-other',
            label: t('pages.inbounds.form.xdrive.yandexOtherServices'),
            children: (
              <Typography.Paragraph type="secondary">
                {t('pages.inbounds.form.xdrive.yandexOtherExplanation')}
              </Typography.Paragraph>
            ),
          },
        ]}
      />
      {TUNING_FIELDS.map(([key, label, placeholder]) => (
        <FormField key={key} label={t(`pages.inbounds.form.xdrive.${label}`)} name={[...BASE, key]}>
          <InputNumber min={0} placeholder={String(placeholder)} style={{ width: '100%' }} />
        </FormField>
      ))}
    </>
  );
}
