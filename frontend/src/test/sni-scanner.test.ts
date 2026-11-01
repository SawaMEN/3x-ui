import { afterEach, describe, expect, it, vi } from 'vitest';
import { HttpUtil, Msg } from '@/utils';
import { runSNIScan, loadSNIHistory } from '@/pages/inbounds/form/security/sni-scanner';
import type { RealityScanResult, SNIJobSnapshot } from '@/generated/types';

const row = {
  host: 'edge.example',
  target: '8.8.8.8:443',
  ip: '8.8.8.8',
  port: 443,
} as RealityScanResult;
const snapshot: SNIJobSnapshot = {
  id: 'scan',
  state: 'running',
  phase: 'discovering',
  source: 'local',
  createdAt: '',
  total: 2,
  completed: 1,
  verified: 0,
  verifyTotal: 0,
  results: [row],
};
afterEach(() => vi.restoreAllMocks());
describe('SNI jobs transport', () => {
  it('delivers intermediate results, polls the selected node and returns final results', async () => {
    const post = vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', snapshot));
    const get = vi
      .spyOn(HttpUtil, 'get')
      .mockResolvedValue(
        new Msg(true, '', { ...snapshot, state: 'done', phase: 'done', completed: 2 }),
      );
    const progress = vi.fn();
    expect(await runSNIScan(7, 'reality', '', 'combined', undefined, progress)).toEqual([row]);
    expect(post).toHaveBeenCalledWith(
      '/panel/api/server/sni/jobs',
      { mode: 'combined', targets: '', purpose: 'reality', nodeId: 7 },
      expect.objectContaining({ headers: { 'Content-Type': 'application/json' } }),
    );
    expect(get).toHaveBeenCalledWith(
      '/panel/api/server/sni/jobs/scan?nodeId=7',
      undefined,
      expect.anything(),
    );
    expect(progress.mock.calls.map(([value]) => value.state)).toEqual(['running', 'done']);
  });
  it('cancels remote work once when aborted', async () => {
    vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', snapshot));
    const remove = vi.spyOn(HttpUtil, 'delete').mockResolvedValue(new Msg(true));
    const controller = new AbortController();
    await expect(
      runSNIScan(7, 'tls', '', 'nearby', controller.signal, () => controller.abort()),
    ).rejects.toMatchObject({ name: 'AbortError' });
    expect(remove).toHaveBeenCalledTimes(1);
    expect(remove).toHaveBeenCalledWith(
      '/panel/api/server/sni/jobs/scan?nodeId=7',
      expect.objectContaining({ silent: true }),
    );
  });
  it('pins saved IP and SNI during recheck and loads history from the selected node', async () => {
    const post = vi
      .spyOn(HttpUtil, 'post')
      .mockResolvedValue(new Msg(true, '', { ...snapshot, state: 'done' }));
    vi.spyOn(HttpUtil, 'get').mockResolvedValue(
      new Msg(true, '', [{ ...snapshot, state: 'done' }]),
    );
    await runSNIScan(4, 'reality', undefined, 'recheck', undefined, undefined, [row]);
    expect(post.mock.calls[0][1]).toMatchObject({
      nodeId: 4,
      recheck: [{ ip: row.ip, host: row.host, port: row.port }],
    });
    expect(await loadSNIHistory(4)).toHaveLength(1);
  });
  it('reports a failed job and malformed start response', async () => {
    const post = vi
      .spyOn(HttpUtil, 'post')
      .mockResolvedValue(
        new Msg(true, '', { ...snapshot, state: 'failed', error: 'queue failure' }),
      );
    await expect(runSNIScan(null, 'tls')).rejects.toThrow('queue failure');
    post.mockResolvedValue(new Msg(true, '', []));
    await expect(runSNIScan(null, 'tls')).rejects.toThrow('could not be started');
  });
});
