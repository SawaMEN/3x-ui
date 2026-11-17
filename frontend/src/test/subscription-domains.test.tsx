import { MemoryRouter } from 'react-router';
import { fireEvent, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { AllSetting } from '@/models/setting';
import SubscriptionDomains from '@/pages/settings/SubscriptionDomains';
import { renderWithProviders } from './test-utils';

describe('subscription public domain settings', () => {
  it('keeps the public HTTPS address independent of the listener port and emits editable settings', () => {
    const update = vi.fn();
    renderWithProviders(
      <MemoryRouter>
        <SubscriptionDomains
          allSetting={new AllSetting({ subPublicURL: 'https://cdn.example.com', subPort: 2096 })}
          updateSetting={update}
        />
      </MemoryRouter>,
    );
    const config = screen.getByRole('textbox', {
      name: 'Caddy reverse proxy configuration',
    }) as HTMLTextAreaElement;
    expect(config.value).toContain('https://cdn.example.com {');
    expect(config.value).toContain('reverse_proxy http://127.0.0.1:2096');
    expect(config.value).toContain('CDN-Cache-Control "no-store"');
    fireEvent.change(screen.getByRole('textbox', { name: 'Public subscription address' }), {
      target: { value: 'https://sub.example.net' },
    });
    expect(update).toHaveBeenCalledWith({ subPublicURL: 'https://sub.example.net' });
    fireEvent.click(screen.getByRole('switch', { name: 'Subscription through CDN' }));
    expect(update).toHaveBeenCalledWith({ subCDNEnable: true });
  });
});

describe('restricted subscription domain selection', () => {
  it('saves the public port and all selected hosts without changing the internal listener', () => {
    const update = vi.fn();
    renderWithProviders(
      <MemoryRouter>
        <SubscriptionDomains
          allSetting={
            new AllSetting({
              subPublicURL: 'https://first.example.com',
              subAllowedDomains: 'second.example.com',
              subPort: 2096,
            })
          }
          updateSetting={update}
        />
      </MemoryRouter>,
    );
    fireEvent.click(screen.getByRole('switch', { name: 'Use only selected domains' }));
    expect(update).toHaveBeenLastCalledWith({
      subRestrictDomains: true,
      subPublicURL: 'https://first.example.com',
      subAllowedDomains: 'second.example.com',
      subCDNEnable: false,
    });
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Public subscription port' }), {
      target: { value: '8443' },
    });
    expect(update).toHaveBeenLastCalledWith({
      subRestrictDomains: true,
      subPublicURL: 'https://first.example.com:8443',
      subAllowedDomains: 'second.example.com',
      subCDNEnable: false,
    });
  });
});
