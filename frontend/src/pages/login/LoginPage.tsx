import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  ConfigProvider,
  Form,
  Input,
  Layout,
  Menu,
  Popover,
  Space,
  Spin,
  message,
} from 'antd';
import {
  KeyOutlined,
  ThunderboltOutlined,
  LockOutlined,
  MoonFilled,
  MoonOutlined,
  SunOutlined,
  TranslationOutlined,
  UserOutlined,
} from '@ant-design/icons';

import { FormProvider, useForm } from 'react-hook-form';
import { HttpUtil, LanguageManager } from '@/utils';
import { FormField, rhfZodValidate } from '@/components/form/rhf';
import { setMessageInstance } from '@/utils/messageBus';
import { pauseAnimationsUntilLeave, useTheme } from '@/hooks/useTheme';
import { LoginFormSchema, TwoFactorCodeSchema, type LoginFormValues } from '@/schemas/login';
import './LoginPage.css';

const HEADLINE_INTERVAL_MS = 2000;

type LoginForm = LoginFormValues;

const basePath = window.X_UI_BASE_PATH || '';

export default function LoginPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, mode, setThemeMode, antdThemeConfig } = useTheme();
  const [messageApi, messageContextHolder] = message.useMessage();

  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const [fetched, setFetched] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [twoFactorEnable, setTwoFactorEnable] = useState(false);
  const [headlineIndex, setHeadlineIndex] = useState(0);
  const methods = useForm<LoginForm>({
    defaultValues: { username: '', password: '', twoFactorCode: '' },
  });
  const [lang, setLang] = useState<string>(() => LanguageManager.getLanguage());

  const headlineWords = useMemo(() => [t('pages.login.hello'), t('pages.login.title')], [t]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      setHeadlineIndex((i) => (i + 1) % headlineWords.length);
    }, HEADLINE_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [headlineWords.length]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const msg = await HttpUtil.post('/getTwoFactorEnable');
      if (cancelled) return;
      if (msg.success) setTwoFactorEnable(!!msg.obj);
      setFetched(true);
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const onSubmit = useCallback(async (values: LoginForm) => {
    setSubmitting(true);
    try {
      const msg = await HttpUtil.post('/login', values);
      if (msg.success) window.location.href = basePath + 'panel/';
    } finally {
      setSubmitting(false);
    }
  }, []);

  const onLangChange = useCallback((next: string) => {
    setLang(next);
    LanguageManager.setLanguage(next);
  }, []);

  const cycleTheme = useCallback(() => {
    pauseAnimationsUntilLeave('login-theme-cycle');
    if (mode === 'light') {
      setThemeMode('dark');
    } else if (mode === 'dark') {
      setThemeMode('ultra-dark');
    } else if (mode === 'ultra-dark') {
      setThemeMode('cyberpunk');
    } else if (mode === 'cyberpunk') {
      setThemeMode('light');
    } else {
      setThemeMode('cyberpunk');
    }
  }, [mode, setThemeMode]);

  const pageClass = useMemo(() => {
    const classes = ['login-app'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const langMenuItems = useMemo(
    () =>
      (LanguageManager.supportedLanguages as { value: string; name: string; icon: string }[]).map(
        (l) => ({
          key: l.value,
          label: (
            <Space size={8}>
              <span aria-hidden="true">{l.icon}</span>
              <span>{l.name}</span>
            </Space>
          ),
        }),
      ),
    [],
  );

  const themeIcon =
    mode === 'cyberpunk' ? (
      <ThunderboltOutlined />
    ) : !isDark ? (
      <SunOutlined />
    ) : !isUltra ? (
      <MoonOutlined />
    ) : (
      <MoonFilled />
    );

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <Layout.Content className="login-content">
          <div className="login-toolbar">
            <Button
              id="login-theme-cycle"
              shape="circle"
              size="large"
              className="toolbar-btn"
              aria-label={t('menu.theme')}
              title={t('menu.theme')}
              icon={themeIcon}
              onClick={cycleTheme}
            />
            <Popover
              rootClassName={isDark ? 'dark' : 'light'}
              placement="bottomRight"
              trigger="click"
              styles={{ content: { padding: 4 } }}
              content={
                <Menu
                  mode="vertical"
                  selectable
                  selectedKeys={[lang]}
                  items={langMenuItems}
                  onClick={({ key }) => onLangChange(key)}
                  style={{ border: 'none', minWidth: 160 }}
                />
              }
            >
              <Button
                shape="circle"
                size="large"
                className="toolbar-btn"
                aria-label={t('pages.settings.language')}
                icon={<TranslationOutlined />}
              />
            </Popover>
          </div>

          <div className="login-wrapper">
            {!fetched ? (
              <div className="login-loading">
                <Spin size="large" />
              </div>
            ) : (
              <div className="login-card">
                <div className="brand">
                  <span className="login-brand-mark" aria-hidden="true">
                    <svg viewBox="0 0 64 64" role="presentation">
                      <path className="login-brand-mark-frame" d="M20 6h24l14 14v24L44 58H20L6 44V20L20 6Z" />
                      <path className="login-brand-mark-core" d="M20 20h24M17 32h30M20 44h24" />
                      <path className="login-brand-mark-x" d="m24 24 16 16M40 24 24 40" />
                      <path className="login-brand-mark-node" d="M6 22h7M51 22h7M6 42h7M51 42h7" />
                      <circle className="login-brand-mark-dot" cx="32" cy="12" r="2" />
                      <circle
                        className="login-brand-mark-dot login-brand-mark-dot-pink"
                        cx="32"
                        cy="52"
                        r="2"
                      />
                    </svg>
                  </span>
                  <span className="brand-name">3X-UI</span>
                  <span className="brand-accent" aria-hidden="true" />
                </div>
                <h2 className="welcome">
                  <b key={headlineIndex}>{headlineWords[headlineIndex]}</b>
                </h2>

                <FormProvider {...methods}>
                  <Form
                    layout="vertical"
                    className="login-form"
                    onFinish={methods.handleSubmit(onSubmit)}
                  >
                    <FormField
                      name="username"
                      label={t('username')}
                      rules={{ validate: rhfZodValidate(LoginFormSchema.shape.username) }}
                    >
                      <Input
                        prefix={<UserOutlined />}
                        autoComplete="username"
                        size="large"
                        placeholder={t('username')}
                        autoFocus
                      />
                    </FormField>

                    <FormField
                      name="password"
                      label={t('password')}
                      rules={{ validate: rhfZodValidate(LoginFormSchema.shape.password) }}
                    >
                      <Input.Password
                        prefix={<LockOutlined />}
                        autoComplete="current-password"
                        size="large"
                        placeholder={t('password')}
                      />
                    </FormField>

                    {twoFactorEnable && (
                      <FormField
                        name="twoFactorCode"
                        label={t('twoFactorCode')}
                        rules={{ validate: rhfZodValidate(TwoFactorCodeSchema) }}
                      >
                        <Input
                          prefix={<KeyOutlined />}
                          autoComplete="one-time-code"
                          size="large"
                          placeholder={t('twoFactorCode')}
                        />
                      </FormField>
                    )}

                    <Form.Item className="submit-row">
                      <Button
                        type="primary"
                        htmlType="submit"
                        loading={submitting}
                        size="large"
                        block
                      >
                        {t('login')}
                      </Button>
                    </Form.Item>
                  </Form>
                </FormProvider>
              </div>
            )}
          </div>
        </Layout.Content>
      </Layout>
    </ConfigProvider>
  );
}
