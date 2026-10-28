import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import ProxyCoreTab from '@/pages/settings/ProxyCoreTab';
import { HttpUtil } from '@/utils';

vi.mock('@tanstack/react-query', () => ({
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}));

const cores = [
  { type: 'sing-box', installed: true, running: true, selected: true, version: 'bundled' },
  { type: 'sing-box-upstream', installed: false, running: false, selected: false, version: '' },
  { type: 'xray', installed: true, running: false, selected: false, version: '26.10.21' },
];
afterEach(() => {
  vi.clearAllMocks();
});
function mockStatus() {
  vi.mocked(HttpUtil.get).mockResolvedValue({
    success: true,
    msg: '',
    obj: { selected: 'sing-box', cores },
  });
}
it('loads actual engine statuses and refreshes after settings save', async () => {
  mockStatus();
  const view = render(<ProxyCoreTab active />);
  await act(async () => {});
  expect(HttpUtil.get).toHaveBeenCalledWith('/panel/api/setting/cores');
  expect(screen.getByText('26.10.21')).toBeTruthy();
  await act(async () => {
    window.dispatchEvent(new Event('xui-core-settings-saved'));
  });
  expect(HttpUtil.get).toHaveBeenCalledTimes(2);
  view.unmount();
});
it('does not request status while hidden', async () => {
  mockStatus();
  const view = render(<ProxyCoreTab active={false} />);
  await act(async () => {
    window.dispatchEvent(new Event('xui-core-settings-saved'));
  });
  expect(HttpUtil.get).not.toHaveBeenCalled();
  view.rerender(<ProxyCoreTab active />);
  await act(async () => {});
  expect(HttpUtil.get).toHaveBeenCalledTimes(1);
  view.unmount();
});
it('installs a separate engine without selecting it', async () => {
  mockStatus();
  vi.mocked(HttpUtil.post).mockResolvedValue({ success: true, msg: '', obj: {} });
  const selected = vi.fn();
  window.addEventListener('xui-core-selected', selected);
  const view = render(<ProxyCoreTab active />);
  await act(async () => {});
  const button = screen.getByRole('button', { name: /Download and install/ });
  await act(async () => {
    fireEvent.click(button);
  });
  expect(HttpUtil.post).toHaveBeenCalledWith(
    '/panel/api/setting/cores/sing-box-upstream/install',
    {},
    { timeout: 330000 },
  );
  expect(selected).not.toHaveBeenCalled();
  window.removeEventListener('xui-core-selected', selected);
  view.unmount();
});
it('publishes the new selection only after a successful switch', async () => {
  mockStatus();
  vi.mocked(HttpUtil.post).mockResolvedValue({ success: true, msg: '', obj: { selected: 'xray' } });
  const selected = vi.fn();
  window.addEventListener('xui-core-selected', selected);
  const view = render(<ProxyCoreTab active />);
  await act(async () => {});
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: /Use/ }));
  });
  expect(HttpUtil.post).toHaveBeenCalledWith(
    '/panel/api/setting/cores/xray/select',
    {},
    { timeout: 330000 },
  );
  expect(selected.mock.calls[0][0].detail).toBe('xray');
  vi.mocked(HttpUtil.post).mockResolvedValue({ success: false, msg: 'invalid config', obj: null });
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: /Use/ }));
  });
  expect(selected).toHaveBeenCalledTimes(1);
  expect(screen.getByText('invalid config')).toBeTruthy();
  window.removeEventListener('xui-core-selected', selected);
  view.unmount();
});
