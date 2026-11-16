import { QueryClientProvider } from '@tanstack/react-query';
import { act, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useOpenFluxInstalled } from '@/api/useOpenFluxInstalled';
import { HttpUtil, Msg } from '@/utils';
import { makeTestQueryClient, renderWithProviders } from './test-utils';

function Status({ nodeId = 0 }: { nodeId?: number }) {
  const installed = useOpenFluxInstalled(true, nodeId);
  return <output>{installed ? 'available' : 'hidden'}</output>;
}
describe('OpenFlux component availability', () => {
  it.each([false, true])('uses the actual component status: %s', async (installed) => {
    vi.mocked(HttpUtil.get).mockResolvedValue(new Msg(true, '', { installed }));
    renderWithProviders(<Status />);
    await waitFor(() => expect(screen.getByText(installed ? 'available' : 'hidden')).toBeTruthy());
    expect(HttpUtil.get).toHaveBeenCalledWith('/panel/api/server/openflux/status', undefined, {
      silent: true,
    });
  });
  it('hides immediately when switching nodes and ignores late replies from the old node', async () => {
    let completeOld: (value: Msg<{ installed: boolean }>) => void = () => {};
    vi.mocked(HttpUtil.get).mockImplementation((path) => {
      if (String(path).endsWith('nodeId=7'))
        return new Promise((resolve) => {
          completeOld = resolve;
        });
      return Promise.resolve(new Msg(true, '', { installed: false }));
    });
    const client = makeTestQueryClient();
    const view = renderWithProviders(<Status nodeId={7} />, { queryClient: client });
    view.rerender(<QueryClientProvider client={client}><Status nodeId={8} /></QueryClientProvider>);
    await act(async () => {
      completeOld(new Msg(true, '', { installed: true }));
    });
    expect(screen.getByText('hidden')).toBeTruthy();
    expect(HttpUtil.get).toHaveBeenCalledWith(
      '/panel/api/server/openflux/status?nodeId=8',
      undefined,
      { silent: true },
    );
  });
  it('does not advertise OpenFlux if the node API fails', async () => {
    vi.mocked(HttpUtil.get).mockRejectedValue(new Error('node unavailable'));
    renderWithProviders(<Status nodeId={9} />);
    await act(async () => {});
    expect(screen.getByText('hidden')).toBeTruthy();
  });
});
