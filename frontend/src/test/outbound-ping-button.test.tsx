import { fireEvent, render, screen } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import OutboundPingButton from '@/pages/singbox/OutboundPingButton';
import { HttpUtil } from '@/utils';
it('measures the automatic selector itself and displays zero milliseconds correctly', async () => {
  vi.mocked(HttpUtil.post).mockResolvedValue({ success: true, msg: '', obj: { delay: 0 } });
  render(<OutboundPingButton tag="sub-auto-7" />);
  fireEvent.click(screen.getByRole('button', { name: 'Пинг' }));
  expect(await screen.findByText('0 мс')).toBeTruthy();
  expect(HttpUtil.post).toHaveBeenCalledWith(
    '/panel/api/server/singbox/outbound/check',
    { tag: 'sub-auto-7', timeout: 10000 },
    { silent: true },
  );
});
it('shows an unavailable node as a readable status instead of throwing', async () => {
  vi.mocked(HttpUtil.post).mockResolvedValue({
    success: false,
    msg: 'Нет доступного подключения',
    obj: null,
  });
  render(<OutboundPingButton tag="group" />);
  fireEvent.click(screen.getByRole('button', { name: 'Пинг' }));
  expect(await screen.findByText('Нет доступного подключения')).toBeTruthy();
});
it('does not test unsaved or unapplied configuration', () => {
  render(<OutboundPingButton tag="new" disabled />);
  expect((screen.getByRole('button', { name: 'Пинг' }) as HTMLButtonElement).disabled).toBe(true);
});
