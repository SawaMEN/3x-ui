import { afterEach, describe, expect, it, vi } from 'vitest';
import { installOpenFlux, ensureOpenFlux } from '@/api/openflux-install';
import { HttpUtil, Msg } from '@/utils';

afterEach(() => vi.restoreAllMocks());

describe('OpenFlux installation', () => {
  it('polls the selected node and reports its actual result', async () => {
    const post = vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', { runId: 'job', state: 'running' }));
    const get = vi.spyOn(HttpUtil, 'get').mockResolvedValue(new Msg(true, '', { runId: 'job', state: 'success' }));
    const result = await installOpenFlux(7);
    expect(result.success).toBe(true);
    expect(post.mock.calls[0][0]).toBe('/panel/api/server/openflux/install?nodeId=7');
    expect(get.mock.calls[0][0]).toBe('/panel/api/server/openflux/install?nodeId=7');
  });
  it('does not install again when the selected node already has OpenFlux', async () => {
    vi.spyOn(HttpUtil, 'get').mockResolvedValue(new Msg(true, '', { installed: true, supported: true }));
    const post = vi.spyOn(HttpUtil, 'post');
    expect((await ensureOpenFlux(4)).success).toBe(true);
    expect(post).not.toHaveBeenCalled();
  });
  it('does not fall back to the master when a remote panel lacks the API', async () => {
    vi.spyOn(HttpUtil, 'get').mockResolvedValue(new Msg(false, 'HTTP 404', null));
    const post = vi.spyOn(HttpUtil, 'post');
    expect((await ensureOpenFlux(4)).success).toBe(false);
    expect(post).not.toHaveBeenCalled();
  });
  it('rejects a lost/replaced job rather than claiming success', async () => {
    vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', { runId: 'old', state: 'running' }));
    vi.spyOn(HttpUtil, 'get').mockResolvedValue(new Msg(true, '', { runId: 'new', state: 'success' }));
    expect((await installOpenFlux()).success).toBe(false);
  });
});
