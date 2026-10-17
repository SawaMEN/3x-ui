import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import SingBoxPage from '@/pages/singbox/SingBoxPage';
import { HttpUtil } from '@/utils';

// Import and subscription handoffs render several dialogs on shared CI CPUs.
vi.setConfig({ testTimeout: 15000 });

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

async function openImporter() {
  fireEvent.click(await screen.findByRole('button', { name: 'Добавить outbound' }));
  fireEvent.click(await screen.findByRole('tab', { name: /JSON/ }));
}

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
    fireEvent.click(await screen.findByText('Каталог готовых списков'));
    await screen.findByText('List catalog');
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
  it('reorders and duplicates routing rules and persists their priority', async () => {
    const first = { action: 'reject', domain: ['ads.test'] };
    const second = { action: 'route', port_range: ['1000:2000'], outbound: 'direct' };
    vi.mocked(HttpUtil.get).mockResolvedValue({
      success: true,
      msg: '',
      obj: {
        config: { ...structuredClone(config), route: { final: 'direct', rules: [first, second] } },
        running: true,
        version: '1.14.0',
        managedSections: ['inbounds'],
      },
    });
    mount('/routing');
    const moveUp = await screen.findAllByTitle('Выше');
    fireEvent.click(moveUp[1]);
    const rows = screen.getAllByRole('row');
    expect(rows[1].textContent).toContain('1000:2000');
    expect(rows[2].textContent).toContain('ads.test');
    fireEvent.click(screen.getAllByTitle('Дублировать')[0]);
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
    expect(JSON.parse((call[1] as { config: string }).config).route.rules).toEqual([
      second,
      second,
      first,
    ]);
  });

  it('searches and disables the original routing rule without changing a different row', async () => {
    const first = { action: 'reject', domain: ['ads.test'] };
    const second = { action: 'route', port: [443], outbound: 'direct' };
    vi.mocked(HttpUtil.get).mockResolvedValue({
      success: true,
      msg: '',
      obj: {
        config: { ...structuredClone(config), route: { final: 'direct', rules: [first, second] } },
        running: true,
        version: '1.14.0',
        managedSections: ['inbounds'],
      },
    });
    mount('/routing');
    const search = await screen.findByLabelText('Поиск правил маршрутизации');
    fireEvent.change(search, { target: { value: '443' } });
    fireEvent.click(screen.getByRole('switch', { name: 'Правило 2' }));
    fireEvent.click(screen.getByText('Save').closest('button')!);
    await waitFor(() => expect(HttpUtil.post).toHaveBeenCalled());
    const call = vi
      .mocked(HttpUtil.post)
      .mock.calls.find(([path]) => path === '/panel/api/setting/singbox/config')!;
    expect(JSON.parse((call[1] as { config: string }).config).route.rules).toEqual([
      first,
      { ...second, enabled: false },
    ]);
  });

  it('edits a tunnel from the same searchable outbound list and keeps route references', async () => {
    const endpoint = {
      type: 'masque-client',
      tag: 'tunnel',
      server: 'vpn.test',
      server_port: 443,
      version: 3,
      path: '/.well-known/masque/ip/{target}/{ipproto}/',
      mtu: 1280,
      system: false,
      tls: { enabled: true },
    };
    vi.mocked(HttpUtil.get).mockResolvedValue({
      success: true,
      msg: '',
      obj: {
        config: {
          ...structuredClone(config),
          endpoints: [endpoint],
          route: { final: 'tunnel', rules: [] },
        },
        running: true,
        version: '1.14.0',
        managedSections: ['inbounds'],
      },
    });
    mount('/outbound');
    fireEvent.change(await screen.findByLabelText('Поиск исходящих подключений'), {
      target: { value: 'tunnel' },
    });
    const row = screen.getByText('tunnel').closest('tr')!;
    fireEvent.click(within(row).getByRole('button', { name: 'Изменить' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(await within(dialog).findByLabelText('Название подключения'), {
      target: { value: 'renamed-tunnel' },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Сохранить изменения' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull(), { timeout: 5000 });
    fireEvent.click(screen.getByText('Save').closest('button')!);
    await waitFor(() => expect(HttpUtil.post).toHaveBeenCalled());
    const call = vi
      .mocked(HttpUtil.post)
      .mock.calls.find(([path]) => path === '/panel/api/setting/singbox/config')!;
    const sent = JSON.parse((call[1] as { config: string }).config);
    expect(sent.endpoints).toMatchObject([{ tag: 'renamed-tunnel', type: 'masque-client' }]);
    expect(sent.route.final).toBe('renamed-tunnel');
    expect(sent.outbounds).toEqual(config.outbounds);
  });

  it('uses a single level of navigation for core, DNS, connections and Gateway', async () => {
    mount();
    await screen.findByText('Логирование');
    expect(screen.getAllByRole('radio').map((item) => item.closest('label')?.textContent)).toEqual([
      'Основные',
      'DNS',
      'Маршрутизация',
      'Подключения',
      'AdBlock',
      'Режим шлюза',
      'Дополнительно',
    ]);
    expect(screen.queryByText('Быстрая настройка')).toBeNull();
    expect(screen.queryByLabelText('Расширенный режим')).toBeNull();
    expect(screen.getByText('Save').closest('button')!.hasAttribute('disabled')).toBe(true);
  });

  it('opens DNS servers directly from the main navigation', async () => {
    mount();
    await screen.findByText('Логирование');
    fireEvent.click(screen.getByRole('radio', { name: 'DNS' }));
    expect(await screen.findByRole('button', { name: 'Добавить DNS' })).toBeTruthy();
    expect(HttpUtil.post).not.toHaveBeenCalled();
  });

  it('exposes rare settings inside expandable expert sections', async () => {
    mount('/singbox#advanced');
    fireEvent.click(await screen.findByText('Кэш-файл и служебные API'));
    await screen.findByText('V2Ray API');
    expect(screen.getByRole('button', { name: 'Редактировать JSON' })).toBeTruthy();
    expect(screen.queryByRole('tab', { name: 'Основные' })).toBeNull();
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
    fireEvent.click(await screen.findAllByText('warn').then((items) => items[items.length - 1]));
    fireEvent.click(screen.getByText('Save').closest('button')!);
    await waitFor(() => expect(HttpUtil.post).toHaveBeenCalled());
    const call = vi
      .mocked(HttpUtil.post)
      .mock.calls.find(([path]) => path === '/panel/api/setting/singbox/config')!;
    const sent = JSON.parse((call[1] as { config: string }).config);
    expect(sent.log).toEqual({ ...config.log, level: 'warn' });
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
    mount('/singbox#advanced');
    await screen.findByText('Дополнительные настройки');
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
    await openImporter();
    fireEvent.change(screen.getByLabelText('Ссылка подключения'), {
      target: { value: 'trojan://pass@proxy.test#Imported' },
    });
    fireEvent.click(screen.getByRole('button', { name: /Импортировать/ }));
    await screen.findByDisplayValue('Imported');
    fireEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: 'Добавить подключение' }),
    );
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
    await openImporter();
    fireEvent.change(screen.getByLabelText('Ссылка подключения'), {
      target: {
        value: 'vless://12345678-1234-1234-1234-123456789abc@proxy.test?type=xhttp#Unsupported',
      },
    });
    fireEvent.click(screen.getByRole('button', { name: /Импортировать/ }));
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
  await openImporter();
  const link = 'mierus://u:p@proxy.test?profile=test&port=2012-2022&protocol=TCP';
  fireEvent.change(screen.getByLabelText('Ссылка подключения'), { target: { value: link } });
  fireEvent.click(screen.getByRole('button', { name: /Импортировать/ }));
  await screen.findByDisplayValue('Mieru imported');
  fireEvent.click(
    within(screen.getByRole('dialog')).getByRole('button', { name: 'Добавить подключение' }),
  );
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

it('has one importer and removes both modal and mask after opening a subscription from it', async () => {
  vi.mocked(HttpUtil.get).mockImplementation(async (url) => ({
    success: true,
    msg: '',
    obj: String(url).endsWith('/outbound-subs')
      ? []
      : {
          config: structuredClone(config),
          running: true,
          version: '1.14.0',
          managedSections: ['inbounds'],
        },
  }));
  mount('/outbound');
  await screen.findByRole('button', { name: 'Добавить outbound' });
  expect(screen.queryByText('Импорт ссылки')).toBeNull();
  await openImporter();
  fireEvent.change(screen.getByLabelText('Ссылка подключения'), {
    target: { value: 'https://provider.test/sub' },
  });
  fireEvent.click(screen.getByRole('button', { name: /Импортировать/ }));
  const drawer = await screen.findByRole('dialog');
  await within(drawer).findByDisplayValue('https://provider.test/sub');
  expect(screen.getAllByRole('dialog')).toHaveLength(1);
  expect(screen.queryByRole('button', { name: 'Добавить подключение' })).toBeNull();
  fireEvent.click(within(drawer).getByRole('button', { name: 'Close' }));
  await waitFor(() => expect(document.querySelector('.ant-drawer')).toBeNull());
  expect(screen.queryByRole('dialog')).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Подписки' }));
  const reopened = await screen.findByRole('dialog');
  fireEvent.click(within(reopened).getByRole('button', { name: 'Close' }));
  await waitFor(() => expect(document.querySelector('.ant-drawer-mask')).toBeNull());
});
