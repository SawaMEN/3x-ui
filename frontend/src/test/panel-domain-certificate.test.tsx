import { beforeEach, expect, it, vi } from 'vitest';
import { fireEvent, screen, waitFor } from '@testing-library/react';
import PanelDomainCertificate from '@/pages/settings/PanelDomainCertificate';
import { DomainFrontStatusSchema } from '@/generated/zod';
import { defaultDomain, defaultDomainConfig } from '@/schemas/domain-front';
import { AllSetting } from '@/models/setting';
import { HttpUtil, Msg } from '@/utils';
import { renderWithProviders } from './test-utils';

const expiry = Math.floor(Date.now() / 1000) + 86400;
function fixture() {
  return DomainFrontStatusSchema.parse({
    config: {
      ...defaultDomainConfig,
      enabled: true,
      domains: [
        { ...defaultDomain, name: 'direct.test', mode: 'direct' },
        { ...defaultDomain, name: 'cdn.test', mode: 'cdn' },
        { ...defaultDomain, name: 'pending.test', mode: 'direct' },
      ],
    },
    running: true,
    error: '',
    existingCertificates: [],
    certificates: [
      {
        name: 'direct.test',
        mode: 'auto',
        state: 'valid',
        autoRenew: true,
        issuer: 'Issuer',
        expiresAt: expiry,
        daysLeft: 1,
        error: '',
        source: 'certmagic',
        renewal: 'certmagic',
        certFile: '/managed/direct.crt',
        keyFile: '/managed/direct.key',
      },
    ],
  });
}
async function selectDomain() {
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Panel domain from direct domains' }));
  await waitFor(() =>
    expect(document.querySelector('.ant-select-item-option[title="direct.test"]')).toBeTruthy(),
  );
  fireEvent.click(document.querySelector('.ant-select-item-option[title="direct.test"]')!);
}
beforeEach(() => {
  vi.mocked(HttpUtil.post).mockClear();
  vi.mocked(HttpUtil.post).mockResolvedValue(
    new Msg(true, '', {
      domain: 'direct.test',
      certFile: '/managed/direct.crt',
      keyFile: '/managed/direct.key',
      expiresAt: expiry,
    }),
  );
});
it('checks the saved local direct pair before requesting a panel switch', async () => {
  const onSwitch = vi.fn();
  renderWithProviders(
    <PanelDomainCertificate
      status={fixture()}
      allSetting={new AllSetting({ webDomain: 'old.test' })}
      disabled={false}
      onSwitch={onSwitch}
    />,
  );
  fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Panel domain from direct domains' }));
  expect(screen.queryByText('cdn.test')).toBeNull();
  expect(
    screen
      .getByText('pending.test — certificate not ready')
      .closest('.ant-select-item-option-disabled'),
  ).toBeTruthy();
  fireEvent.click(document.querySelector('.ant-select-item-option[title="direct.test"]')!);
  expect(onSwitch).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', { name: 'Switch domain' }));
  await waitFor(() =>
    expect(onSwitch).toHaveBeenCalledWith({
      webDomain: 'direct.test',
      webCertFile: '/managed/direct.crt',
      webKeyFile: '/managed/direct.key',
    }),
  );
  expect(HttpUtil.post).toHaveBeenCalledWith(
    '/panel/api/domains/certificates/panel-domain',
    { domain: 'direct.test' },
    { silent: true },
  );
  expect(HttpUtil.post).toHaveBeenCalledTimes(1);
});
it('does not switch with unsaved domain changes', () => {
  renderWithProviders(
    <PanelDomainCertificate
      status={fixture()}
      allSetting={new AllSetting()}
      disabled
      onSwitch={vi.fn()}
    />,
  );
  expect(screen.getByRole('button', { name: 'Switch domain' })).toHaveProperty('disabled', true);
  expect(
    screen.getByText('Save domain changes first and wait for the certificate to be ready.'),
  ).toBeTruthy();
  expect(HttpUtil.post).not.toHaveBeenCalled();
});
it('rejects a certificate response belonging to another node or hostname', async () => {
  const onSwitch = vi.fn();
  vi.mocked(HttpUtil.post).mockResolvedValue(
    new Msg(true, '', {
      domain: 'other.test',
      nodeId: 9,
      certFile: '/remote/a.crt',
      keyFile: '/remote/a.key',
      expiresAt: expiry,
    }),
  );
  renderWithProviders(
    <PanelDomainCertificate
      status={fixture()}
      allSetting={new AllSetting()}
      disabled={false}
      onSwitch={onSwitch}
    />,
  );
  await selectDomain();
  fireEvent.click(screen.getByRole('button', { name: 'Switch domain' }));
  expect(await screen.findByText('Unable to prepare the panel domain switch.')).toBeTruthy();
  expect(onSwitch).not.toHaveBeenCalled();
});
it('does not restart when the domain and pair are already selected', async () => {
  renderWithProviders(
    <PanelDomainCertificate
      status={fixture()}
      allSetting={
        new AllSetting({
          webDomain: 'direct.test',
          webCertFile: '/managed/direct.crt',
          webKeyFile: '/managed/direct.key',
        })
      }
      disabled={false}
      onSwitch={vi.fn()}
    />,
  );
  await selectDomain();
  expect(screen.getByRole('button', { name: 'Switch domain' })).toHaveProperty('disabled', true);
  expect(HttpUtil.post).not.toHaveBeenCalled();
});
