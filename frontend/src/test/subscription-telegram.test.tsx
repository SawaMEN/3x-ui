import { fireEvent, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import SubTelegramTab from '@/pages/sub/SubTelegramTab';
import { telegramProfiles } from '@/pages/sub/telegramProfiles';
import { renderWithProviders } from './test-utils';

const link = 'tg://proxy?server=node.example&port=443&secret=ee1234';
const web = 'tg://webproxy?server=web.example&secret=dd1234';

describe('subscription Telegram profiles', () => {
  it('merges node profiles with inbound links and deduplicates query parameter order', () => {
    const profiles = telegramProfiles(
      { proxies: [{ name: 'Remote node', host: 'node.example', port: 443, tls: true, link }] },
      [
        'vless://id@vpn.example:443',
        'tg://proxy?secret=ee1234&port=443&server=node.example',
        web,
        'tg://proxy?server=broken.example&secret=1234',
        'javascript:alert(1)',
      ],
    );
    expect(profiles).toHaveLength(2);
    const copy = vi.fn();
    const open = vi.fn();
    renderWithProviders(
      <SubTelegramTab
        profiles={profiles}
        loading={false}
        error={false}
        onCopy={copy}
        onOpen={open}
      />,
    );
    expect(screen.getByText('Remote node')).toBeTruthy();
    expect(screen.getByText('web.example', { selector: '.sub-row-title' })).toBeTruthy();
    fireEvent.click(screen.getAllByRole('button', { name: 'Open in Telegram' })[0]);
    expect(open).toHaveBeenCalledWith(link);
    fireEvent.click(screen.getAllByRole('button', { name: 'Copy' })[0]);
    expect(copy).toHaveBeenCalledWith(link);
  });
});
