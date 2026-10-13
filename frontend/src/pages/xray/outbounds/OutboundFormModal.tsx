import AdditionalOutboundEditor from './AdditionalOutboundEditor';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Form,
  Input,
  InputNumber,
  Modal,
  Radio,
  Select,
  Space,
  Tabs,
  Typography,
  message,
} from 'antd';
import { Controller, FormProvider, useForm, useWatch } from 'react-hook-form';
import { FinalMaskField, SniffingField } from '@/lib/xray/forms/fields';
import { FormField, rhfZodValidate } from '@/components/form/rhf';
import { JsonEditor } from '@/components/form';
import { Base64, HttpUtil, Wireguard } from '@/utils';
import { formValuesToWirePayload, rawOutboundToFormValues } from '@/lib/xray/outbound-form-adapter';
import { parseOutboundLink } from '@/lib/xray/outbound-link-parser';
import { parseNativeOutboundLink } from '@/lib/xray/native-outbound-link-parser';
import { outboundBasicConversionLoss } from '@/lib/xray/outbound-loss-check';
import { XMUX_FRESH_DEFAULTS } from '@/schemas/protocols/stream/xhttp';
import {
  OutboundFormBaseSchema,
  VlessOutboundFormSettingsSchema,
  type OutboundFormValues,
} from '@/schemas/forms/outbound-form';
import {
  canEnableReality,
  canEnableStream,
  canEnableTls,
  canEnableTlsFlow,
} from '@/lib/xray/protocol-capabilities';

import {
  FLOW_OPTIONS,
  HYSTERIA_NETWORK_OPTION,
  NETWORK_OPTIONS,
  PROTOCOL_OPTIONS,
  SERVER_PROTOCOLS,
  SINGBOX_NATIVE_OUTBOUND_PROTOCOL_SET,
  TARGET_STRATEGY_OPTIONS,
} from './outbound-form-constants';
import {
  applyNetworkChange,
  buildAddModeValues,
  hysteriaStreamSlice,
  newStreamSlice,
} from './outbound-form-helpers';
import {
  buildNativeSingBoxOutbound,
  normalizeOutboundJsonForPanel,
  normalizeOutboundJsonImport,
} from './singbox-native-outbound';
import {
  BlackholeFields,
  DnsFields,
  FreedomFields,
  HttpFields,
  ServerTarget,
  ShadowsocksFields,
  SocksFields,
  TrojanFields,
  VlessFields,
  VmessFields,
  WireguardFields,
} from './protocols';
import { AmneziawgFields } from './protocols';
import {
  GrpcForm,
  HttpUpgradeForm,
  HysteriaForm,
  MuxForm,
  KcpForm,
  RawForm,
  SockoptForm,
  WsForm,
  XhttpForm,
} from './transport';
import { RealityForm, TlsForm } from './security';
import './OutboundFormModal.css';

type StreamValue = OutboundFormValues['streamSettings'];

interface OutboundFormModalProps {
  open: boolean;
  outbound: Record<string, unknown> | null;
  existingTags: string[];
  dialerProxyTags?: string[];
  onClose: () => void;
  onOpenSubscriptions?: () => void;
  onConfirm: (outbound: Record<string, unknown>) => void;
}

function supportsBasicOutboundForm(protocol: unknown): boolean {
  if (typeof protocol !== 'string') return false;
  const normalized = protocol.toLowerCase();
  if (SINGBOX_NATIVE_OUTBOUND_PROTOCOL_SET.has(normalized)) return false;
  return PROTOCOL_OPTIONS.some((option) => option.value === normalized);
}

export default function OutboundFormModal({
  open,
  outbound: outboundProp,
  existingTags,
  dialerProxyTags,
  onClose,
  onOpenSubscriptions,
  onConfirm,
}: OutboundFormModalProps) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const methods = useForm<OutboundFormValues>({ defaultValues: buildAddModeValues() });
  const [activeKey, setActiveKey] = useState('1');
  const [jsonText, setJsonText] = useState('');
  const [jsonDirty, setJsonDirty] = useState(false);
  const [linkInput, setLinkInput] = useState('');

  const isEdit = outboundProp != null;
  const title = isEdit ? 'Редактировать outbound' : 'Новый outbound';
  const okText = isEdit ? t('pages.clients.submitEdit') : t('create');

  const tag = (useWatch({ control: methods.control, name: 'tag' }) ?? '') as string;
  const protocol = (useWatch({ control: methods.control, name: 'protocol' }) ?? 'vless') as string;
  const network = (useWatch({ control: methods.control, name: 'streamSettings.network' }) ??
    '') as string;
  const security = (useWatch({ control: methods.control, name: 'streamSettings.security' }) ??
    'none') as string;
  const reverseTag = useWatch({ control: methods.control, name: 'settings.reverseTag' });
  const wgSecretKey = useWatch({ control: methods.control, name: 'settings.secretKey' }) as
    | string
    | undefined;

  const streamAllowed = canEnableStream({ protocol });
  const tlsAllowed = canEnableTls({ protocol, streamSettings: { network, security } });
  const realityAllowed = canEnableReality({ protocol, streamSettings: { network, security } });
  const flow = useWatch({ control: methods.control, name: 'settings.flow' });
  useEffect(() => {
    if (protocol === 'vless' && flow === 'xtls-rprx-vision') {
      const seed = methods.getValues('settings.testseed');
      if (!seed?.length || seed.every((value) => value == null)) {
        methods.setValue('settings.testseed', [900, 500, 900, 256]);
      }
    }
  }, [flow, protocol, methods]);

  const tlsFlowAllowed = canEnableTlsFlow({ protocol, streamSettings: { network, security } });

  /*
   * Import a core-native URI, legacy URI, single JSON outbound or base64 JSON
   * without forcing native settings through the typed Xray form.
   * Multiple JSON outbounds belong in the subscription/outbound list importer.
   */
  async function importLink() {
    const link = linkInput.trim();
    if (!link) return;
    let parsed: Record<string, unknown> | null = null;
    try {
      const isJson = link.startsWith('{') || link.startsWith('[');
      const decodeJson = (raw: string): Record<string, unknown> => {
        const normalized = normalizeOutboundJsonImport(JSON.parse(raw) as unknown);
        if (normalized.length !== 1)
          throw new Error('Вставьте один outbound. Для списка используйте импорт подписки.');
        return normalized[0];
      };
      if (isJson) {
        parsed = decodeJson(link);
      } else if (link.startsWith('fptn:') || link.startsWith('openflux://')) {
        const result = await HttpUtil.post(
          '/panel/api/server/externalvpn/parse-link',
          { link },
          { headers: { 'Content-Type': 'application/json' } },
        );
        if (!result?.success) throw new Error(result?.msg || 'Не удалось разобрать ссылку');
        parsed = result.obj as Record<string, unknown>;
      } else {
        parsed = parseOutboundLink(link) ?? parseNativeOutboundLink(link);
        if (!parsed && !link.includes('://')) {
          try {
            parsed = decodeJson(Base64.decode(link));
          } catch {
            /* Not a JSON blob. */
          }
        }
      }
    } catch (error) {
      messageApi.error((error as Error).message || 'Не удалось импортировать ссылку');
      return;
    }
    if (!parsed) {
      messageApi.error('Неизвестный формат. Для протоколов без share URI используйте JSON ядра.');
      return;
    }
    const currentTag = methods.getValues('tag');
    if (!parsed.tag && currentTag) parsed.tag = currentTag;
    const lostOnBasic = supportsBasicOutboundForm(parsed.protocol)
      ? outboundBasicConversionLoss(parsed)
      : null;
    if (!supportsBasicOutboundForm(parsed.protocol) || lostOnBasic) {
      if (lostOnBasic) {
        messageApi.warning(`Параметр ${lostOnBasic} сохранён в JSON без преобразования.`);
      }
      setJsonText(JSON.stringify(parsed, null, 2));
      setJsonDirty(false);
      setLinkInput('');
      messageApi.success('Параметры загружены в JSON без потери дополнительных полей');
      switchTab('2');
      return;
    }
    const next = rawOutboundToFormValues(parsed);
    methods.reset(next);
    setJsonText(JSON.stringify(formValuesToWirePayload(next), null, 2));
    setJsonDirty(false);
    setLinkInput('');
    messageApi.success('Настройки соединения импортированы');
    switchTab('1');
  }

  useEffect(() => {
    if (!open) return;
    setLinkInput('');

    // Native/future sing-box outbounds may not have a typed Basic form yet.
    // Preserve their JSON byte-for-byte semantically instead of passing them
    // through rawOutboundToFormValues(), whose typed adapter intentionally
    // understands only the Xray-compatible form protocols.
    if (
      outboundProp &&
      (!supportsBasicOutboundForm(outboundProp.protocol) ||
        outboundBasicConversionLoss(outboundProp))
    ) {
      methods.reset(buildAddModeValues());
      setActiveKey('2');
      setJsonText(JSON.stringify(outboundProp, null, 2));
      setJsonDirty(false);
      return;
    }

    const initial = outboundProp ? rawOutboundToFormValues(outboundProp) : buildAddModeValues();
    methods.reset(initial);
    setActiveKey('1');
    setJsonText(JSON.stringify(formValuesToWirePayload(initial), null, 2));
    setJsonDirty(false);
  }, [open, outboundProp, methods]);

  useEffect(() => {
    if (!streamAllowed) return;
    /*
     * Wireguard dials its own UDP — only finalmask/sockopt apply, never a
     * transport. Don't seed network 'tcp'; clear a leftover one (from a
     * protocol switch) so the transmission/security blocks stay hidden.
     */
    if (protocol === 'wireguard') {
      if (network) methods.setValue('streamSettings', { security: 'none' } as StreamValue);
      return;
    }
    if (network) return;
    methods.setValue('streamSettings', {
      ...newStreamSlice('tcp'),
      security: 'none',
    } as StreamValue);
  }, [streamAllowed, network, protocol, methods]);

  useEffect(() => {
    if (protocol !== 'hysteria') return;
    if (network === 'hysteria' && security === 'tls') return;
    const existing = (methods.getValues('streamSettings') ?? {}) as Record<string, unknown>;
    const slice = hysteriaStreamSlice();
    if (existing.hysteriaSettings) slice.hysteriaSettings = existing.hysteriaSettings;
    if (existing.tlsSettings) slice.tlsSettings = existing.tlsSettings;
    methods.setValue('streamSettings', slice as StreamValue);
  }, [protocol, network, security, methods]);

  useEffect(() => {
    if (protocol !== 'wireguard') return;
    const sk = (wgSecretKey ?? '').trim();
    if (!sk) {
      methods.setValue('settings.pubKey', '');
      return;
    }
    try {
      const { publicKey } = Wireguard.generateKeypair(sk);
      methods.setValue('settings.pubKey', publicKey);
    } catch {
      methods.setValue('settings.pubKey', '');
    }
  }, [protocol, wgSecretKey, methods]);

  useEffect(() => {
    /* eslint-disable-next-line react-hooks/incompatible-library */
    const sub = methods.watch((_value, { name, type }) => {
      if (name !== 'protocol' || type !== 'change') return;
      const nextProtocol = String(methods.getValues('protocol') ?? '').toLowerCase();
      if (SINGBOX_NATIVE_OUTBOUND_PROTOCOL_SET.has(nextProtocol)) {
        const native = buildNativeSingBoxOutbound(nextProtocol, methods.getValues('tag') ?? '');
        setJsonText(JSON.stringify(native, null, 2));
        setJsonDirty(false);
        setActiveKey('2');
        return;
      }
      const next = rawOutboundToFormValues({ protocol: nextProtocol });
      methods.setValue('settings', next.settings);
      if (nextProtocol === 'hysteria') {
        methods.setValue('streamSettings', hysteriaStreamSlice() as StreamValue);
      } else if ((methods.getValues('streamSettings.network') ?? '') === 'hysteria') {
        methods.setValue('streamSettings', {
          ...newStreamSlice('tcp'),
          security: 'none',
        } as StreamValue);
      }
    });
    return () => sub.unsubscribe();
  }, [methods]);

  function onSecurityChange(next: string) {
    const stream = (methods.getValues('streamSettings') ?? {}) as Record<string, unknown>;
    const cleaned = { ...stream };
    delete cleaned.tlsSettings;
    delete cleaned.realitySettings;
    if (next === 'tls') {
      cleaned.tlsSettings = {
        serverName: '',
        alpn: [],
        fingerprint: '',
        echConfigList: '',
        verifyPeerCertByName: '',
        pinnedPeerCertSha256: '',
      };
    } else if (next === 'reality') {
      cleaned.realitySettings = {
        publicKey: '',
        fingerprint: 'chrome',
        serverName: '',
        shortId: '',
        spiderX: '',
        mldsa65Verify: '',
      };
    }
    cleaned.security = next;
    methods.setValue('streamSettings', cleaned as StreamValue);
    // Vision is valid only with supported TLS/Reality transport.
    if (protocol === 'vless' && next === 'none' && flow === 'xtls-rprx-vision')
      methods.setValue('settings.flow', '');
  }

  /*
   * Network change cascade: swap the per-network sub-key (tcpSettings,
   * wsSettings, etc.) so the DU branch matches. Preserve security if
   * the new network supports it, otherwise force back to 'none'.
   */
  function onNetworkChange(next: string) {
    const stream = (methods.getValues('streamSettings') ?? {}) as Record<string, unknown>;
    const updated = applyNetworkChange(protocol, stream, next);
    methods.setValue('streamSettings', updated as StreamValue);
    if (
      protocol === 'vless' &&
      flow === 'xtls-rprx-vision' &&
      (next !== 'tcp' || updated.security === 'none')
    )
      methods.setValue('settings.flow', '');
  }

  function onXmuxToggle(checked: boolean) {
    if (!checked) return;
    const existing = methods.getValues('streamSettings.xhttpSettings.xmux');
    const hasValues = existing && typeof existing === 'object' && Object.keys(existing).length > 0;
    if (hasValues) return;
    methods.setValue('streamSettings.xhttpSettings.xmux', { ...XMUX_FRESH_DEFAULTS });
  }

  const duplicateTag = useMemo(() => {
    const myTag = tag.trim();
    if (!myTag) return false;
    if (isEdit && (outboundProp?.tag as string | undefined) === myTag) return false;
    return (existingTags || []).includes(myTag);
  }, [tag, existingTags, isEdit, outboundProp]);

  /*
   * Bridge form <-> JSON tab: when leaving the JSON tab back to Basic, push
   * any edits into form state. Native/future sing-box protocols intentionally
   * remain in JSON mode so the typed Xray adapter cannot rewrite their shape.
   */
  function applyJsonToForm(): boolean {
    const raw = jsonText.trim();
    if (!raw) return true;
    let parsed: Record<string, unknown>;
    try {
      const value = JSON.parse(raw) as unknown;
      if (!value || typeof value !== 'object' || Array.isArray(value)) {
        messageApi.error('JSON outbound must be an object');
        return false;
      }
      parsed = normalizeOutboundJsonForPanel(value as Record<string, unknown>);
    } catch (e) {
      messageApi.error(`JSON: ${(e as Error).message}`);
      return false;
    }
    if (!supportsBasicOutboundForm(parsed.protocol)) {
      messageApi.info('Native sing-box outbounds are edited in JSON mode');
      return false;
    }
    const lostOnBasic = outboundBasicConversionLoss(parsed);
    if (lostOnBasic) {
      messageApi.warning(`Параметр ${lostOnBasic} недоступен в визуальной форме. JSON сохранён.`);
      return false;
    }
    if (!jsonDirty) return true;
    const next = rawOutboundToFormValues(parsed);
    methods.reset(next);
    setJsonDirty(false);
    return true;
  }

  function switchTab(key: string) {
    if (typeof document !== 'undefined') {
      (document.activeElement as HTMLElement | null)?.blur?.();
    }
    setActiveKey(key);
  }

  function onTabChange(key: string) {
    if (key === activeKey) return;
    if (key === '2') {
      const values = methods.getValues();
      setJsonText(JSON.stringify(formValuesToWirePayload(values), null, 2));
      setJsonDirty(false);
      switchTab(key);
      return;
    }
    if (key === '1' && activeKey === '2') {
      if (!applyJsonToForm()) return;
    }
    switchTab(key);
  }

  async function onOk() {
    let payload: Record<string, unknown>;
    let tagValue: string;

    if (activeKey === '2') {
      const raw = jsonText.trim();
      if (!raw) return;
      let parsed: Record<string, unknown>;
      try {
        const value = JSON.parse(raw) as unknown;
        if (!value || typeof value !== 'object' || Array.isArray(value)) {
          messageApi.error('JSON outbound must be an object');
          return;
        }
        parsed = normalizeOutboundJsonForPanel(value as Record<string, unknown>);
      } catch (e) {
        messageApi.error(`JSON: ${(e as Error).message}`);
        return;
      }
      const protocolValue =
        typeof parsed.protocol === 'string' ? parsed.protocol.trim().toLowerCase() : '';
      if (!protocolValue) {
        messageApi.error('Protocol is required');
        return;
      }
      tagValue = typeof parsed.tag === 'string' ? parsed.tag.trim() : '';
      payload = { ...parsed, protocol: protocolValue };
      setJsonDirty(false);
    } else {
      if (!(await methods.trigger())) return;
      const values = methods.getValues();
      tagValue = (values.tag ?? '').trim();
      payload = formValuesToWirePayload(values);
    }

    if (!tagValue) {
      messageApi.error(t('pages.xray.outboundForm.tagRequired'));
      return;
    }
    if (tagValue.startsWith('_bl_')) {
      messageApi.error(t('pages.xray.balancer.reservedPrefix'));
      return;
    }
    const isDuplicateTag =
      (existingTags || []).includes(tagValue) &&
      !(isEdit && (outboundProp?.tag as string | undefined) === tagValue);
    if (isDuplicateTag) {
      messageApi.error('Tag already used by another outbound');
      return;
    }

    payload.tag = tagValue;
    onConfirm(payload);
  }

  return (
    <>
      {messageContextHolder}
      <Modal
        open={open}
        title={title}
        okText={okText}
        cancelText={t('close')}
        mask={{ closable: false }}
        width={960}
        className="outbound-form-modal"
        onOk={onOk}
        onCancel={onClose}
        destroyOnHidden
      >
        <div className="outbound-form-heading">
          <Typography.Text strong>Параметры исходящего подключения</Typography.Text>
          <Typography.Text type="secondary">
            Автоматическая синхронизация протокола, транспорта и защиты. JSON доступен для всех
            параметров ядра.
          </Typography.Text>
        </div>
        <FormProvider {...methods}>
          <Form
            colon={false}
            labelCol={{ md: { span: 8 } }}
            wrapperCol={{ md: { span: 14 } }}
            labelWrap
          >
            <Tabs
              className="outbound-form-tabs"
              activeKey={activeKey}
              onChange={onTabChange}
              items={[
                {
                  key: '1',
                  label: 'Визуальный редактор',
                  children: (
                    <>
                      <FormField
                        label={t('protocol')}
                        name="protocol"
                        rules={{ validate: rhfZodValidate(OutboundFormBaseSchema.shape.tag) }}
                      >
                        <Select id="protocol" options={PROTOCOL_OPTIONS} />
                      </FormField>

                      <Controller
                        control={methods.control}
                        name="tag"
                        rules={{ required: 'pages.xray.outboundForm.tagRequired' }}
                        render={({ field, fieldState }) => {
                          const errorMessage = fieldState.error?.message
                            ? t(fieldState.error.message, {
                                defaultValue: fieldState.error.message,
                              })
                            : '';
                          return (
                            <Form.Item
                              label={t('pages.xray.outbound.tag')}
                              required
                              validateStatus={
                                errorMessage ? 'error' : duplicateTag ? 'warning' : undefined
                              }
                              help={
                                errorMessage ||
                                (duplicateTag
                                  ? t('pages.xray.outboundForm.tagDuplicate')
                                  : undefined)
                              }
                            >
                              <Input
                                value={field.value}
                                onChange={(e) => field.onChange(e.target.value)}
                                onBlur={field.onBlur}
                                ref={field.ref}
                                placeholder={t('pages.xray.outboundForm.tagPlaceholder')}
                              />
                            </Form.Item>
                          );
                        }}
                      />

                      <FormField label={t('pages.xray.outbound.sendThrough')} name="sendThrough">
                        <Input placeholder={t('pages.xray.outboundForm.localIpPlaceholder')} />
                      </FormField>

                      {/* Freedom's own card owns the strategy — the core migrates this
                          root key into the same sockopt value, so two knobs would race. */}
                      {protocol !== 'freedom' && (
                        <FormField
                          label={t('pages.xray.outbound.targetStrategy')}
                          name="targetStrategy"
                          tooltip={t('pages.xray.outboundForm.targetStrategyHint')}
                        >
                          <Select allowClear placeholder="AsIs" options={TARGET_STRATEGY_OPTIONS} />
                        </FormField>
                      )}

                      {SERVER_PROTOCOLS.has(protocol) && <ServerTarget />}
                      {protocol === 'vmess' && <VmessFields />}
                      {protocol === 'vless' && <VlessFields />}
                      {protocol === 'trojan' && <TrojanFields />}
                      {protocol === 'shadowsocks' && <ShadowsocksFields />}
                      {protocol === 'http' && <HttpFields />}
                      {protocol === 'socks' && <SocksFields />}

                      {protocol === 'blackhole' && <BlackholeFields />}
                      {protocol === 'dns' && <DnsFields />}

                      {protocol === 'freedom' && <FreedomFields />}

                      {protocol === 'vless' && reverseTag && (
                        <Controller
                          control={methods.control}
                          name="settings.reverseSniffing"
                          render={({ field }) => (
                            <SniffingField
                              value={field.value}
                              onChange={field.onChange}
                              enableLabel={t('pages.xray.outboundForm.reverseSniffing')}
                            />
                          )}
                        />
                      )}

                      {protocol === 'wireguard' && <WireguardFields />}
                      {protocol === 'amneziawg' && <AmneziawgFields />}

                      {streamAllowed && network && (
                        <>
                          <Form.Item label={t('transmission')}>
                            <Select
                              value={network}
                              onChange={onNetworkChange}
                              options={
                                protocol === 'hysteria'
                                  ? [HYSTERIA_NETWORK_OPTION]
                                  : NETWORK_OPTIONS.filter(
                                      (option) =>
                                        option.value !== 'kcp' ||
                                        ['vless', 'vmess', 'trojan'].includes(protocol),
                                    )
                              }
                            />
                          </Form.Item>

                          {network === 'tcp' && <RawForm />}
                          {network === 'kcp' && <KcpForm />}

                          {network === 'ws' && <WsForm />}

                          {network === 'grpc' && <GrpcForm />}

                          {network === 'httpupgrade' && <HttpUpgradeForm />}

                          {network === 'xhttp' && <XhttpForm onXmuxToggle={onXmuxToggle} />}

                          {network === 'hysteria' && <HysteriaForm />}
                        </>
                      )}

                      {tlsFlowAllowed && (
                        <FormField label={t('pages.clients.flow')} name={['settings', 'flow']}>
                          <Select
                            allowClear
                            placeholder={t('none')}
                            options={[{ value: '', label: t('none') }, ...FLOW_OPTIONS]}
                          />
                        </FormField>
                      )}
                      {tlsFlowAllowed && flow === 'xtls-rprx-vision' && (
                        <>
                          <FormField
                            label={t('pages.xray.outboundForm.visionTestpre')}
                            name={['settings', 'testpre']}
                            rules={{
                              validate: rhfZodValidate(
                                VlessOutboundFormSettingsSchema.shape.testpre,
                              ),
                            }}
                          >
                            <InputNumber min={0} max={64} style={{ width: '100%' }} />
                          </FormField>
                          <Form.Item label={t('pages.inbounds.form.visionTestseed')}>
                            <Space.Compact block>
                              {[0, 1, 2, 3].map((i) => (
                                <FormField
                                  key={i}
                                  name={['settings', 'testseed', i]}
                                  noStyle
                                  rules={{
                                    validate: (_value, values) => {
                                      const result =
                                        VlessOutboundFormSettingsSchema.shape.testseed.safeParse(
                                          values.settings.testseed,
                                        );
                                      return result.success || result.error.issues[0].message;
                                    },
                                  }}
                                >
                                  <InputNumber
                                    min={i === 2 ? 0 : 1}
                                    max={65536}
                                    style={{ width: '25%' }}
                                  />
                                </FormField>
                              ))}
                            </Space.Compact>
                          </Form.Item>
                        </>
                      )}

                      {streamAllowed && network && (
                        <Form.Item label={t('security')}>
                          <Radio.Group
                            value={security}
                            buttonStyle="solid"
                            onChange={(e) => onSecurityChange(e.target.value as string)}
                          >
                            {network !== 'hysteria' && (
                              <Radio.Button value="none">{t('none')}</Radio.Button>
                            )}
                            {tlsAllowed && <Radio.Button value="tls">TLS</Radio.Button>}
                            {realityAllowed && <Radio.Button value="reality">Reality</Radio.Button>}
                          </Radio.Group>
                        </Form.Item>
                      )}

                      {security === 'tls' && tlsAllowed && <TlsForm />}

                      {security === 'reality' && realityAllowed && <RealityForm />}

                      {((streamAllowed && network) ||
                        !streamAllowed ||
                        protocol === 'wireguard') && (
                        <SockoptForm
                          outboundTags={dialerProxyTags ?? existingTags}
                          showDomainStrategy={protocol !== 'freedom'}
                        />
                      )}

                      <Controller
                        control={methods.control}
                        name="streamSettings.finalmask"
                        render={({ field }) => (
                          <FinalMaskField
                            key={`${protocol}:${network}`}
                            value={field.value}
                            onChange={field.onChange}
                            network={network}
                            protocol={protocol}
                          />
                        )}
                      />

                      <MuxForm protocol={protocol} network={network} />
                    </>
                  ),
                },
                {
                  key: '2',
                  label: 'JSON и импорт',
                  children: (
                    <Space
                      orientation="vertical"
                      size={10}
                      style={{ width: '100%', marginTop: 10 }}
                    >
                      <div className="outbound-form-import">
                        <Typography.Text strong>Импорт конфигурации</Typography.Text>
                        <Typography.Text type="secondary">
                          Вставьте ссылку протокола, один JSON outbound или base64 JSON. Параметры
                          native sing-box сохраняются в JSON без преобразования.
                        </Typography.Text>
                        <Input.Search
                          value={linkInput}
                          placeholder="vless://, anytls://, tuic://, hysteria://, JSON..."
                          enterButton="Импортировать"
                          onChange={(e) => setLinkInput(e.target.value)}
                          onSearch={importLink}
                        />
                        {onOpenSubscriptions && (
                          <Typography.Link onClick={onOpenSubscriptions}>
                            Управление подписками: добавить URL, обновить и настроить автообновление
                          </Typography.Link>
                        )}
                      </div>
                      <Alert
                        type="info"
                        showIcon
                        title="JSON является источником истины для расширенных протоколов"
                        description="Если у протокола нет стандартной ссылки, вставьте его полный native JSON. Неизвестные ядру поля проверяются при сохранении сервером."
                      />
                      <AdditionalOutboundEditor
                        text={jsonText}
                        onChange={(next) => {
                          setJsonText(next);
                          setJsonDirty(true);
                        }}
                      />
                      <JsonEditor
                        value={jsonText}
                        onChange={(next) => {
                          setJsonText(next);
                          setJsonDirty(true);
                        }}
                        minHeight="360px"
                        maxHeight="600px"
                      />
                    </Space>
                  ),
                },
              ]}
            />
          </Form>
        </FormProvider>
      </Modal>
    </>
  );
}
