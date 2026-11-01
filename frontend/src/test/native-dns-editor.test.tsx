import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import NativeDnsServers from '@/pages/singbox/NativeDnsServers';
import NativeDnsRules from '@/pages/singbox/NativeDnsRules';

const config = {
  outbounds: [{ type: 'direct', tag: 'direct' }],
  dns: { final: 'system', servers: [{ type: 'local', tag: 'system' }], rules: [] },
};
describe('native DNS forms', () => {
  it('adds a verified DoH server from a pasted URL without editing JSON', async () => {
    const onChange = vi.fn();
    render(<NativeDnsServers config={config} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: 'Добавить DNS' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText('Название'), {
      target: { value: 'secure-dns' },
    });
    const type = within(dialog).getByLabelText('Тип');
    fireEvent.mouseDown(type.closest('.ant-select')!.querySelector('.ant-select-content')!);
    fireEvent.click(await screen.findByText('https'));
    fireEvent.change(within(dialog).getByLabelText('Адрес сервера'), {
      target: { value: 'https://dns.test:8443/query' },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Сохранить' }));
    await waitFor(() => expect(onChange).toHaveBeenCalled());
    expect(onChange.mock.calls[0][0].dns).toMatchObject({
      final: 'system',
      servers: [
        { tag: 'system' },
        {
          type: 'https',
          tag: 'secure-dns',
          server: 'dns.test',
          server_port: 8443,
          path: '/query',
          tls: { enabled: true },
        },
      ],
    });
  });
  it('updates DNS rule and bootstrap references when a server is renamed', async () => {
    const onChange = vi.fn();
    const current = {
      ...config,
      route: { default_domain_resolver: { server: 'system', strategy: 'ipv4_only' } },
      dns: { ...config.dns, rules: [{ domain: ['lan'], server: 'system' }] },
    };
    render(<NativeDnsServers config={current} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: 'Изменить' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText('Название'), { target: { value: 'renamed' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Сохранить' }));
    await waitFor(() => expect(onChange).toHaveBeenCalled());
    expect(onChange.mock.calls[0][0]).toMatchObject({
      route: { default_domain_resolver: { server: 'renamed', strategy: 'ipv4_only' } },
      dns: { final: 'renamed', rules: [{ server: 'renamed' }] },
    });
  });
  it('creates an ordered DNS rule from domain fields', async () => {
    const onChange = vi.fn();
    render(<NativeDnsRules config={config} onChange={onChange} />);
    fireEvent.click(screen.getByRole('button', { name: 'Добавить правило' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText('Домены и поддомены'), {
      target: { value: 'example.com\nexample.org' },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Сохранить' }));
    await waitFor(() => expect(onChange).toHaveBeenCalled());
    expect(onChange.mock.calls[0][0].dns.rules).toEqual([
      { action: 'route', server: 'system', domain_suffix: ['example.com', 'example.org'] },
    ]);
  });
});
