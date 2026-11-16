import { HttpUtil, Msg } from '@/utils';

type Installation = {
  runId: string;
  state: 'idle' | 'running' | 'success' | 'failed';
  error?: string;
};

// The job runs on the selected panel; only short authenticated requests are
// polled, so a large download does not hit the panel's HTTP write timeout.
export async function installOpenFlux(nodeId = 0, signal?: AbortSignal): Promise<Msg<unknown>> {
  const url = '/panel/api/server/openflux/install' + (nodeId > 0 ? '?nodeId=' + nodeId : '');
  let result = await HttpUtil.post<Installation>(url, undefined, {
    signal,
    timeout: 15000,
    silent: true,
  });
  const runId = result.obj?.runId;
  if (!result.success || !runId)
    return new Msg(false, result.msg || 'OpenFlux installation did not start', null);
  const deadline = Date.now() + 11 * 60 * 1000;
  while (result.obj?.state === 'running') {
    if (signal?.aborted) throw new DOMException('Aborted', 'AbortError');
    if (Date.now() >= deadline)
      return new Msg(false, 'OpenFlux installation timed out; check component status', null);
    await new Promise((resolve) => setTimeout(resolve, 1000));
    if (signal?.aborted) throw new DOMException('Aborted', 'AbortError');
    result = await HttpUtil.get<Installation>(url, undefined, {
      signal,
      timeout: 15000,
      silent: true,
    });
    if (!result.success)
      return new Msg(false, result.msg || 'Unable to read OpenFlux installation status', null);
    if (result.obj?.runId !== runId)
      return new Msg(false, 'OpenFlux installation status changed; check component status', null);
  }
  return result.obj?.state === 'success'
    ? new Msg(true, '', result.obj)
    : new Msg(false, result.obj?.error || result.msg || 'OpenFlux installation failed', null);
}

export async function ensureOpenFlux(nodeId = 0): Promise<Msg<unknown>> {
  try {
    const url = '/panel/api/server/openflux/status' + (nodeId > 0 ? '?nodeId=' + nodeId : '');
    const result = await HttpUtil.get<{
      installed: boolean;
      supported: boolean;
      unsupportedReason?: string;
    }>(url, undefined, { timeout: 15000, silent: true });
    if (!result.success || !result.obj || typeof result.obj.installed !== 'boolean')
      return new Msg(
        false,
        result.msg || 'Unable to check OpenFlux on the selected node; update that panel first',
        null,
      );
    if (result.obj.installed) return new Msg(true, '', result.obj);
    if (result.obj.supported === false)
      return new Msg(
        false,
        result.obj.unsupportedReason || 'OpenFlux is unsupported on this platform',
        null,
      );
    return await installOpenFlux(nodeId);
  } catch (error) {
    return new Msg(
      false,
      error instanceof Error ? error.message : 'OpenFlux installation failed',
      null,
    );
  }
}
