import { afterEach, describe, expect, it, vi } from 'vitest';
import { waitForPanelRestart } from '@/pages/settings/panel-url';

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});
const response = (instance: string, status = 200) =>
  new Response('', { status, headers: { 'X-Panel-Instance': instance } });
describe('panel restart readiness', () => {
  it('does not mistake the previous listener for a completed restart', async () => {
    vi.useFakeTimers();
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(response('old'))
      .mockResolvedValue(response('new'));
    vi.stubGlobal('fetch', fetcher);
    let ready = false;
    const result = waitForPanelRestart(
      'https://panel.test/new/',
      'https://panel.test/old/',
      'old',
    ).then((value) => {
      ready = value;
      return value;
    });
    await vi.advanceTimersByTimeAsync(100);
    expect(ready).toBe(false);
    await vi.advanceTimersByTimeAsync(500);
    expect(await result).toBe(true);
    expect(fetcher).toHaveBeenCalledTimes(2);
  });
  it('probes the existing origin when the destination is blocked by mixed-content rules', async () => {
    vi.useFakeTimers();
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(response('old'))
      .mockResolvedValue(response('new', 403));
    vi.stubGlobal('fetch', fetcher);
    const result = waitForPanelRestart('http://new.test:8080/', 'https://old.test/', 'old');
    await vi.advanceTimersByTimeAsync(500);
    expect(await result).toBe(true);
    expect(fetcher.mock.calls.every(([url]) => url === 'https://old.test/')).toBe(true);
  });
  it('waits for the old listener to close before moving to a changed port', async () => {
    vi.useFakeTimers();
    const fetcher = vi
      .fn()
      .mockResolvedValueOnce(response('old'))
      .mockRejectedValue(new TypeError('closed'));
    vi.stubGlobal('fetch', fetcher);
    const result = waitForPanelRestart('http://panel.test:8080/', 'http://panel.test:2053/', 'old');
    await vi.runAllTimersAsync();
    expect(await result).toBe(true);
    expect(fetcher).toHaveBeenCalledTimes(3);
  });
});
