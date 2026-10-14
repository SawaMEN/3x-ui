import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import SingBoxPage from '@/pages/singbox/SingBoxPage';
import { HttpUtil } from '@/utils';

vi.mock('@/layouts/AppSidebar', () => ({ default: () => null }));
vi.mock('@/pages/adblock/AdBlockTab', () => ({ default: () => null }));
vi.mock('@/hooks/useTheme', () => ({
  useTheme: () => ({ antdThemeConfig: {}, isDark: false, isUltra: false }),
}));

const config = {
  log: { level: 'info', output: '/tmp/sing-box.log' },
  dns: {
    servers: [{ type: 'local', tag: 'local' }],
    final: 'local',
    rules: [{ domain_suffix: ['lan'], server: 'local' }],
  },
  route: { final: 'direct', rules: [{ action: 'reject', domain: ['ads.test'] }] },
  outbounds: [{ type: 'direct', tag: 'direct' }],
  inbounds: [{ type: 'vless', tag: 'managed' }],
  services: [{ type: 'resolved' }],
};

beforeEach(() => {
  vi.mocked(HttpUtil.get).mockResolvedValue({
    success: true,
    msg: '',
    obj: {
      config: structuredClone(config),
      running: true,
      version: '1.14.0',
      configSource: 'disk',
      managedSections: ['inbounds'],
    },
  });
  vi.mocked(HttpUtil.post).mockClear();
});

function mount(path = '/singbox') {
  render(
    <MemoryRouter initialEntries={[path]}>
      <SingBoxPage />
    </MemoryRouter>,
  );
}

describe('simple sing-box page', () => {
  it('exposes the catalog in current core settings and saves a managed native rule', async () => {
    vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
      success: true,
      msg: '',
      obj: url.endsWith('/routingCatalog/categories')
        ? [{ id: 'youtube', token: 'geosite:youtube', available: true }]
        : url.endsWith('/routingCatalog/list')
          ? []
          : {
              config: structuredClone(config),
              running: false,
              version: '1.14.0',
              managedSections: ['inbounds'],
            },
    }));
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/singbox#routing']}>
          <SingBoxPage />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    const catalogTab = await screen.findByText('List catalog');
    fireEvent.click(catalogTab);
    const row = (await screen.findByText('geosite:youtube', {}, { timeout: 5000 })).closest('tr')!;
    fireEvent.click(within(row).getByText('Assign route').closest('button')!);
    fireEvent.click((await screen.findByText('Add routing rule')).closest('button')!);
    fireEvent.click(screen.getByText('Save').closest('button')!);
    await waitFor(() =>
      expect(HttpUtil.post).toHaveBeenCalledWith(
        '/panel/api/setting/singbox/config',
        expect.anything(),
      ),
    );
    const call = vi
      .mocked(HttpUtil.post)
      .mock.calls.find(([path]) => path === '/panel/api/setting/singbox/config')!;
    const sent = JSON.parse(String((call[1] as { config: string }).config));
    expect(sent.route.rules).toEqual([
      config.route.rules[0],
      expect.objectContaining({
        routingListCategory: 'youtube',
        action: 'route',
        outbound: 'direct',
      }),
    ]);
    expect(sent.inbounds).toBeUndefined();
    expect(sent.services).toBeUndefined();
  });
  it('starts with simple controls and only exposes expert sections after switching', async () => {
    mount();
    await screen.findByText('Быстрая настройка');
    expect(screen.queryByText('Endpoints')).toBeNull();
    expect(screen.queryByText('V2Ray API')).toBeNull();
    expect(screen.getByText('Save').closest('button')!.hasAttribute('disabled')).toBe(true);
    fireEvent.click(screen.getByLabelText('Расширенный режим'));
    expect(await screen.findByText('Endpoints')).toBeTruthy();
  });

  it('shows current settings and opens DNS from the overview', async () => {
    mount();
    await screen.findByText('Быстрая настройка');
    expect(screen.getByText('Системный DNS')).toBeTruthy();
    expect(screen.getByText('Журнал событий')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Настроить: DNS' }));
    await screen.findByText('Определение адресов сайтов (DNS)');
    expect(screen.getByLabelText('Выбор IP-адреса')).toBeTruthy();
    expect(HttpUtil.post).not.toHaveBeenCalled();
  });

  it('exposes rare settings inside expandable expert sections', async () => {
    mount('/singbox#advanced');
    fireEvent.click(await screen.findByText('Кэш-файл и служебные API'));
    await screen.findByText('V2Ray API');
    fireEvent.click(screen.getByText('Журнал, время и схема'));
    await screen.findByText('Логирование');
  });

  it('saves simple edits with custom rules intact and omits panel-managed sections', async () => {
    mount('/singbox#dns');
    fireEvent.click(await screen.findByLabelText('Кэш DNS'));
    fireEvent.click(screen.getByText('Save').closest('button')!);
    await waitFor(() =>
      expect(HttpUtil.post).toHaveBeenCalledWith(
        '/panel/api/setting/singbox/config',
        expect.anything(),
      ),
    );
    const call = vi
      .mocked(HttpUtil.post)
      .mock.calls.find(([path]) => path === '/panel/api/setting/singbox/config');
    expect(call).toBeDefined();
    const sent = JSON.parse((call![1] as { config: string }).config);
    const { inbounds: _inbounds, services: _services, ...unmanaged } = config;
    expect(sent).toEqual({ ...unmanaged, dns: { ...config.dns, disable_cache: true } });
  });

  it('saves log detail while preserving the custom file and managed accounting settings', async () => {
    mount();
    const select = await screen.findByLabelText('Подробность журнала');
    fireEvent.mouseDown(select.closest('.ant-select')!.querySelector('.ant-select-content')!);
    fireEvent.click(await screen.findByText('Предупреждения и ошибки — меньше записей'));
    fireEvent.click(screen.getByText('Save').closest('button')!);
    await waitFor(() => expect(HttpUtil.post).toHaveBeenCalled());
    const call = vi
      .mocked(HttpUtil.post)
      .mock.calls.find(([path]) => path === '/panel/api/setting/singbox/config')!;
    const sent = JSON.parse((call[1] as { config: string }).config);
    expect(sent.log).toEqual({ ...config.log, level: 'warn', disabled: false });
    expect(sent.dns).toEqual(config.dns);
    expect(sent.route).toEqual(config.route);
    expect(sent.inbounds).toBeUndefined();
    expect(sent.services).toBeUndefined();
  });

  it('asks before discarding unsaved edits', async () => {
    mount('/singbox#dns');
    await screen.findByText('Определение адресов сайтов (DNS)');
    fireEvent.click(screen.getByLabelText('Кэш DNS'));
    const loads = vi.mocked(HttpUtil.get).mock.calls.length;
    fireEvent.click(screen.getByText('Refresh').closest('button')!);
    await screen.findAllByText('Отменить несохранённые изменения?');
    fireEvent.click(screen.getByText('Продолжить настройку').closest('button')!);
    expect(HttpUtil.get).toHaveBeenCalledTimes(loads);
    expect(HttpUtil.post).not.toHaveBeenCalled();
  });

  it('asks before resetting automatic settings', async () => {
    mount();
    await screen.findByText('Быстрая настройка');
    fireEvent.click(screen.getByLabelText('Расширенный режим'));
    fireEvent.click(screen.getByText('Reset to generated settings').closest('button')!);
    await screen.findAllByText('Вернуть автоматические настройки sing-box?');
    expect(HttpUtil.post).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText('Отмена').closest('button')!);
  });
  it('imports standard Trojan links with TLS enabled', async () => {
    vi.mocked(HttpUtil.post).mockResolvedValue({
      success: true,
      msg: '',
      obj: {
        outbounds: [
          {
            type: 'trojan',
            tag: 'Imported',
            server: 'proxy.test',
            server_port: 443,
            password: 'pass',
            tls: { enabled: true, server_name: 'proxy.test' },
          },
        ],
        endpoints: [],
      },
    });
    mount('/singbox#outbound');
    fireEvent.click(await screen.findByText('Импорт ссылки'));
    fireEvent.change(screen.getByPlaceholderText('vless://...'), {
      target: { value: 'trojan://pass@proxy.test#Imported' },
    });
    fireEvent.click(screen.getByText('Добавить').closest('button')!);
    await screen.findByText('Imported');
    fireEvent.click(screen.getByText('Save').closest('button')!);
    await waitFor(() =>
      expect(HttpUtil.post).toHaveBeenCalledWith(
        '/panel/api/setting/singbox/config',
        expect.anything(),
      ),
    );
    const call = vi
      .mocked(HttpUtil.post)
      .mock.calls.find(([path]) => path === '/panel/api/setting/singbox/config')!;
    const sent = JSON.parse((call[1] as { config: string }).config);
    expect(sent.outbounds).toContainEqual({
      type: 'trojan',
      tag: 'Imported',
      server: 'proxy.test',
      server_port: 443,
      password: 'pass',
      tls: { enabled: true, server_name: 'proxy.test' },
    });
  });

  it('keeps links rejected by the kernel out of the configuration', async () => {
    vi.mocked(HttpUtil.post).mockResolvedValue({
      success: false,
      msg: 'Ядро не приняло ссылку',
      obj: null,
    });
    mount('/singbox#outbound');
    fireEvent.click(await screen.findByText('Импорт ссылки'));
    fireEvent.change(screen.getByPlaceholderText('vless://...'), {
      target: {
        value: 'vless://12345678-1234-1234-1234-123456789abc@proxy.test?type=xhttp#Unsupported',
      },
    });
    fireEvent.click(screen.getByText('Добавить').closest('button')!);
    await screen.findByText('Ядро не приняло ссылку');
    expect(screen.queryByText('Unsupported')).toBeNull();
    expect(screen.getByText('Save').closest('button')!.hasAttribute('disabled')).toBe(true);
    expect(HttpUtil.post).toHaveBeenCalledWith(
      '/panel/api/setting/singbox/import-link',
      expect.objectContaining({ batch: true }),
    );
    expect(
      vi
        .mocked(HttpUtil.post)
        .mock.calls.some(([path]) => path === '/panel/api/setting/singbox/config'),
    ).toBe(false);
  });
});

it.each(['/outbound', '/routing'])('opens the matching native section for %s', async (path) => {
  mount(path);
  expect(
    await screen.findByText(
      path === '/outbound'
        ? 'Standard core settings · Исходящие'
        : 'Standard core settings · Маршрутизация',
    ),
  ).toBeTruthy();
});

it('decodes Mieru through the pure import API and saves native port bindings', async () => {
  const outbound = {
    type: 'mieru',
    tag: 'Mieru imported',
    server: 'proxy.test',
    username: 'u',
    password: 'p',
    portBindings: [{ protocol: 'TCP', portRange: '2012-2022' }],
    multiplexing: 'HIGH',
    handshake_mode: 'NO_WAIT',
  };
  vi.mocked(HttpUtil.post).mockImplementation(async (path) =>
    path === '/panel/api/setting/singbox/import-link'
      ? { success: true, msg: '', obj: { outbounds: [outbound], endpoints: [] } }
      : { success: true, msg: '', obj: null },
  );
  mount('/outbound');
  fireEvent.click(await screen.findByText('Импорт ссылки'));
  const link = 'mierus://u:p@proxy.test?profile=test&port=2012-2022&protocol=TCP';
  fireEvent.change(screen.getByPlaceholderText('vless://...'), { target: { value: link } });
  fireEvent.click(screen.getByText('Добавить').closest('button')!);
  await screen.findByText('Mieru imported');
  expect(HttpUtil.post).toHaveBeenCalledWith('/panel/api/setting/singbox/import-link', {
    link,
    batch: true,
  });
  fireEvent.click(screen.getByText('Save').closest('button')!);
  await waitFor(() =>
    expect(HttpUtil.post).toHaveBeenCalledWith(
      '/panel/api/setting/singbox/config',
      expect.anything(),
    ),
  );
  const call = vi
    .mocked(HttpUtil.post)
    .mock.calls.find(([path]) => path === '/panel/api/setting/singbox/config')!;
  expect(JSON.parse((call[1] as { config: string }).config).outbounds).toContainEqual(outbound);
});
