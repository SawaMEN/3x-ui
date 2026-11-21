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
    expect(screen.getByRole('link', { name: 'Download · Hiddify' }).getAttribute('href')).toMatch(
      /Hiddify-MacOS\.dmg$/,
    );

    fireEvent.click(screen.getByRole('radio', { name: 'Windows' }));
    expect(screen.getByRole('radio', { name: 'Windows', checked: true })).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Download · Hiddify' }).getAttribute('href')).toMatch(
      /Hiddify-Windows-Setup-x64\.exe$/,
    );
    expect(screen.getByRole('link', { name: 'Add · Hiddify' }).getAttribute('href')).toBe(
      `hiddify://import/${subUrl}#Profile`,
    );
  });
});
