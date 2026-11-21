import { fireEvent, render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import SubAppsTab from '@/pages/sub/SubAppsTab';
import SubLinksTab from '@/pages/sub/SubLinksTab';
import { renderWithProviders } from './test-utils';

const subUrl = 'https://sub.example.com:8443/sub/abc';

describe('subscription tabs', () => {
  it('shows each AWG profile once in the second tab and gives its QR the native config', async () => {
    const config = '[Interface]\nPrivateKey = test\n# AWG North\n[Peer]\nPublicKey = test\n';
    const awgLink = `vpn://${Buffer.from(config).toString('base64url')}`;
    vi.stubGlobal('__SUB_PAGE_DATA__', {
      subUrl,
      enabled: true,
      links: ['vless://test@vpn.example:443#Core', awgLink, awgLink],
    });
    const canvas = vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);

    try {
      const [{ default: SubPage }] = await Promise.all([
        import('@/pages/sub/SubPage'),
        import('@/pages/sub/SubAmneziaWGTab'),
        import('@/pages/sub/SubConfigsTab'),
      ]);
      renderWithProviders(<SubPage />);
      const tab = screen.getByRole('tab', { name: 'AmneziaWG' });
      expect(screen.getAllByRole('tab')[1]).toBe(tab);
      fireEvent.click(screen.getByRole('tab', { name: 'Apps' }));
      fireEvent.click(await screen.findByRole('button', { name: 'Configs · AmneziaWG' }));
      const profile = await screen.findByText('AWG North');
      expect(screen.getAllByText('AWG North')).toHaveLength(1);
      expect(profile.closest('[aria-expanded]')?.getAttribute('aria-expanded')).toBe('false');
      fireEvent.click(profile);
      expect(profile.closest('[aria-expanded]')?.getAttribute('aria-expanded')).toBe('true');
      const panel = screen.getByRole('tabpanel', { name: 'AmneziaWG' });
      expect(within(panel).getAllByRole('button', { name: 'QR' })).toHaveLength(1);
      fireEvent.click(within(panel).getByRole('button', { name: 'QR' }));
      const dialog = await screen.findByRole('dialog');
      expect(dialog.querySelector('canvas')?.width).toBe(360);
      const text = within(dialog).getByRole<HTMLTextAreaElement>('textbox', { name: 'Configs' });
      expect(text.value).toBe(config);
      fireEvent.click(within(dialog).getByRole('button', { name: 'Close' }));
      fireEvent.click(screen.getByRole('tab', { name: /^Configs/ }));
      const core = await screen.findByRole('tabpanel', { name: /^Configs/ });
      expect(await within(core).findByText('Core', { selector: '.sub-row-title' })).toBeTruthy();
      expect(within(core).queryByText('AWG North')).toBeNull();
    } finally {
      canvas.mockRestore();
      vi.unstubAllGlobals();
    }
  }, 15_000);

  it('shows the main QR code first and copies the subscription address', () => {
    const onCopy = vi.fn();
    const { container } = render(
      <SubLinksTab subUrl={subUrl} subJsonUrl="" subClashUrl="" onCopy={onCopy} />,
    );

    const qrCard = screen.getByRole('region', { name: 'Scan with your phone' });
    expect(container.firstElementChild?.firstElementChild).toBe(qrCard);
    expect(qrCard.querySelector('.sub-qr-code svg')).not.toBeNull();
    fireEvent.click(within(qrCard).getByRole('button', { name: /^copy url$/i }));
    expect(onCopy).toHaveBeenCalledWith(subUrl);
  });

  it('starts on the detected platform and updates download links when switched', () => {
    const onOpenAmneziaWG = vi.fn();
    render(
      <SubAppsTab
        subUrl={subUrl}
        profileName="Profile"
        initialPlatform="macos"
        onOpenAmneziaWG={onOpenAmneziaWG}
      />,
    );

    expect(screen.getAllByRole('radio')).toHaveLength(5);
    expect(screen.getByRole('radio', { name: 'macOS', checked: true })).toBeTruthy();
    expect(screen.getByRole('link', { name: 'App Store · Hiddify' }).getAttribute('href')).toMatch(
      /id6596777532\?platform=mac$/,
    );
    expect(screen.getByRole('link', { name: '.dmg · Happ' }).getAttribute('href')).toMatch(
      /Happ\.macOS\.universal\.dmg$/,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Configs · AmneziaWG' }));
    expect(onOpenAmneziaWG).toHaveBeenCalledOnce();

    fireEvent.click(screen.getByRole('radio', { name: 'Windows' }));
    expect(screen.getByRole('radio', { name: 'Windows', checked: true })).toBeTruthy();
    expect(
      screen.getByRole('link', { name: 'Microsoft Store · Hiddify' }).getAttribute('href'),
    ).toBe('https://apps.microsoft.com/detail/9ndgwm32bnxk');
    expect(screen.getByRole('link', { name: '.exe · Hiddify' }).getAttribute('href')).toMatch(
      /Hiddify-Windows-Setup-x64\.exe$/,
    );
    expect(screen.getByRole('link', { name: 'Add · Hiddify' }).getAttribute('href')).toBe(
      `hiddify://import/${subUrl}#Profile`,
    );
  });

  it('shows the collapsed region guide only on Apple platforms and changes its steps with the platform', () => {
    render(<SubAppsTab subUrl={subUrl} profileName="Profile" initialPlatform="ios" />);

    const guide = screen.getByText('App unavailable in the App Store?').closest('details');
    if (!guide) throw new Error('Apple region details are missing');
    expect(guide.open).toBe(false);
    fireEvent.click(within(guide).getByText('App unavailable in the App Store?'));
    expect(guide.open).toBe(true);
    expect(within(guide).getByText('Open Settings and tap your name.')).toBeTruthy();
    expect(
      within(guide).getByRole('link', { name: 'Apple Support instructions' }).getAttribute('href'),
    ).toBe('https://support.apple.com/en-us/118283');

    fireEvent.click(screen.getByRole('radio', { name: 'macOS' }));
    expect(within(guide).getByText('Open the Apple Music or Apple TV app.')).toBeTruthy();

    fireEvent.click(screen.getByRole('radio', { name: 'Android' }));
    expect(screen.queryByText('App unavailable in the App Store?')).toBeNull();
    expect(screen.getByRole('link', { name: 'Google Play · Hiddify' })).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Google Play · Happ' })).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Google Play · AmneziaWG' })).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Configs · AmneziaWG' })).toBeNull();
  });
});
