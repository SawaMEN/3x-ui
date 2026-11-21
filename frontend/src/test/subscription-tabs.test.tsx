import { fireEvent, render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import SubAppsTab from '@/pages/sub/SubAppsTab';
import SubLinksTab from '@/pages/sub/SubLinksTab';

const subUrl = 'https://sub.example.com:8443/sub/abc';

describe('subscription tabs', () => {
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
    render(<SubAppsTab subUrl={subUrl} profileName="Profile" initialPlatform="macos" />);

    expect(screen.getAllByRole('radio')).toHaveLength(5);
    expect(screen.getByRole('radio', { name: 'macOS', checked: true })).toBeTruthy();
    expect(screen.getByRole('link', { name: 'App Store · Hiddify' }).getAttribute('href')).toMatch(
      /id6596777532\?platform=mac$/,
    );
    expect(screen.getByRole('link', { name: '.dmg · Happ' }).getAttribute('href')).toMatch(
      /Happ\.macOS\.universal\.dmg$/,
    );

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
  });
});
