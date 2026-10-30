import { useMutation, useQueryClient } from '@tanstack/react-query';

import { HttpUtil, Msg } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { keys } from '@/api/queryKeys';
import type { NodeRecord } from '@/api/queries/useNodesQuery';
import { ProbeResultSchema, type ProbeResult } from '@/schemas/node';

export type { ProbeResult };

export interface NodeUpdateResult {
  id: number;
  name?: string;
  ok: boolean;
  runId?: string;
  error?: string;
}

export interface RemotePanelUpdateStatus {
  runId: string;
  state: 'pending' | 'success' | 'failed';
  exitCode: number;
  message?: string;
  logFile?: string;
  finishedAt?: number;
}

export interface RemoteInboundOption {
  tag: string;
  remark?: string;
  protocol?: string;
  port?: number;
}

export function useNodeMutations() {
  const queryClient = useQueryClient();
  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: keys.nodes.root() });
    queryClient.invalidateQueries({ queryKey: keys.inbounds.options() });
  };

  const createMut = useMutation({
    mutationFn: (payload: Partial<NodeRecord>) => HttpUtil.post('/panel/api/nodes/add', payload),
    onSuccess: (msg) => {
      if (msg?.success) invalidate();
    },
  });

  const updateMut = useMutation({
    mutationFn: ({ id, payload }: { id: number; payload: Partial<NodeRecord> }) =>
      HttpUtil.post(`/panel/api/nodes/update/${id}`, payload),
    onSuccess: (msg) => {
      if (msg?.success) invalidate();
    },
  });

  const removeMut = useMutation({
    mutationFn: (id: number) => HttpUtil.post(`/panel/api/nodes/del/${id}`),
    onSuccess: (msg) => {
      if (msg?.success) invalidate();
    },
  });

  const setEnableMut = useMutation({
    mutationFn: ({ id, enable }: { id: number; enable: boolean }) =>
      HttpUtil.post(`/panel/api/nodes/setEnable/${id}`, { enable }),
    onSuccess: (msg) => {
      if (msg?.success) invalidate();
    },
  });

  const probeMut = useMutation({
    mutationFn: async (id: number): Promise<Msg<ProbeResult>> => {
      const raw = await HttpUtil.post(`/panel/api/nodes/probe/${id}`);
      return parseMsg(raw, ProbeResultSchema, 'nodes/probe');
    },
    onSuccess: (msg) => {
      if (msg?.success) invalidate();
    },
  });

  const updatePanelsMut = useMutation({
    mutationFn: async ({ ids, dev }: { ids: number[]; dev: boolean }): Promise<Msg<NodeUpdateResult[]>> => {
      // The panel HTTP server has a 30-second WriteTimeout. Each node launch
      // can wait up to 25 seconds, so large requests must be split into
      // bounded fanout batches rather than timing out after the first group.
      const results: NodeUpdateResult[] = [];
      const batchSize = 24;
      for (let start = 0; start < ids.length; start += batchSize) {
        const batch = ids.slice(start, start + batchSize);
        const response = await HttpUtil.post<NodeUpdateResult[]>(
          '/panel/api/nodes/updatePanel',
          { ids: batch, dev },
          {
            headers: { 'Content-Type': 'application/json' },
            timeout: 28_000,
            silent: true,
          },
        );
        if (!response?.success || !Array.isArray(response.obj)) {
          const error = response?.msg || 'Failed to start remote panel update';
          results.push(...batch.map((id) => ({ id, ok: false, error })));
          continue;
        }
        const byId = new Map(response.obj.map((result) => [result.id, result]));
        for (const id of batch) {
          results.push(byId.get(id) ?? { id, ok: false, error: 'Node returned no result' });
        }
      }
      return new Msg<NodeUpdateResult[]>(true, '', results);
    },
    onSuccess: (msg) => {
      if (msg?.success) invalidate();
    },
  });

  return {
    create: (payload: Partial<NodeRecord>) => createMut.mutateAsync(payload),
    update: (id: number, payload: Partial<NodeRecord>) => updateMut.mutateAsync({ id, payload }),
    remove: (id: number) => removeMut.mutateAsync(id),
    setEnable: (id: number, enable: boolean) => setEnableMut.mutateAsync({ id, enable }),
    probe: (id: number) => probeMut.mutateAsync(id),
    updatePanels: (ids: number[], dev: boolean): Promise<Msg<NodeUpdateResult[]>> =>
      updatePanelsMut.mutateAsync({ ids, dev }),
    testConnection: async (payload: Partial<NodeRecord>): Promise<Msg<ProbeResult>> => {
      const raw = await HttpUtil.post('/panel/api/nodes/test', payload);
      return parseMsg(raw, ProbeResultSchema, 'nodes/test');
    },
    fetchFingerprint: (payload: Partial<NodeRecord>): Promise<Msg<string>> =>
      HttpUtil.post<string>('/panel/api/nodes/certFingerprint', payload),
    fetchInbounds: (payload: Partial<NodeRecord>): Promise<Msg<RemoteInboundOption[]>> =>
      HttpUtil.post<RemoteInboundOption[]>('/panel/api/nodes/inbounds', payload),
  };
}
