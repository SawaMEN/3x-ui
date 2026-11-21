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
    fireEvent.click(screen.getByRole('button', { name: 'CDN and reverse proxy' }));
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
  it('edits the public address and restriction independently of the local listener', () => {
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
    expect(update).toHaveBeenLastCalledWith({ subRestrictDomains: true });
    fireEvent.change(screen.getByRole('textbox', { name: 'Public subscription address' }), {
      target: { value: 'https://first.example.com:8443' },
    });
    expect(update).toHaveBeenLastCalledWith({ subPublicURL: 'https://first.example.com:8443' });
    expect(screen.queryByRole('spinbutton', { name: 'Public subscription port' })).toBeNull();
    expect(screen.getByRole('combobox', { name: 'Additional subscription domains' })).toBeTruthy();
  });
});
