import { afterEach, expect, it, vi } from 'vitest';
vi.mock('@/api/http-init', () => ({ httpRequest: vi.fn() }));
import { httpRequest } from '@/api/http-init';
import { HttpUtil } from '@/utils';
const request = vi.mocked(httpRequest);
afterEach(() => {
  vi.restoreAllMocks();
  request.mockReset();
});
it.each([204, 205])('accepts an empty HTTP %s response', (status) => {
  expect(
    HttpUtil._respToMsg({ ok: true, status, statusText: 'No Content', data: '' }).success,
  ).toBe(true);
});
it('rejects a string false in the API success flag', () => {
  expect(
    HttpUtil._respToMsg({ ok: true, status: 200, statusText: 'OK', data: { success: 'false' } })
      .success,
  ).toBe(false);
});
it.each(['get', 'post', 'delete'] as const)(
  '%s does not notify intentional cancellation',
  async (method) => {
    const controller = new AbortController();
    controller.abort();
    request.mockRejectedValueOnce(new DOMException('Aborted', 'AbortError'));
    const handler = vi.spyOn(HttpUtil, '_handleMsg').mockImplementation(() => {});
    const options = { signal: controller.signal };
    const result =
      method === 'get'
        ? await HttpUtil.get('/test', undefined, options)
        : method === 'post'
          ? await HttpUtil.post('/test', {}, options)
          : await HttpUtil.delete('/test', options);
    expect(result.success).toBe(false);
    expect(result.msg).toBe('');
    expect(handler).not.toHaveBeenCalled();
  },
);
it.each([null, undefined, 'offline'])('handles non-Error failures: %s', async (thrown) => {
  request.mockRejectedValueOnce(thrown);
  const result = await HttpUtil.get('/test', undefined, { silent: true });
  expect(result.success).toBe(false);
  expect(result.msg).toBe(thrown === 'offline' ? 'offline' : 'Request failed');
});
it('silent DELETE produces neither console nor toast errors', async () => {
  request.mockRejectedValueOnce(new Error('offline'));
  const log = vi.spyOn(console, 'error').mockImplementation(() => {});
  const handler = vi.spyOn(HttpUtil, '_handleMsg').mockImplementation(() => {});
  expect((await HttpUtil.delete('/test', { silent: true })).success).toBe(false);
  expect(log).not.toHaveBeenCalled();
  expect(handler).not.toHaveBeenCalled();
});

it('honors params in GET options and lets explicit params override them', async () => {
  request.mockResolvedValue({ ok: true, status: 200, statusText: 'OK', data: { success: true } });
  await HttpUtil.get('/test', undefined, { params: { page: 2 }, silent: true });
  expect(request.mock.calls[0][3]?.params).toEqual({ page: 2 });
  await HttpUtil.get('/test', { page: 3 }, { params: { page: 2 }, silent: true });
  expect(request.mock.calls[1][3]?.params).toEqual({ page: 3 });
});
