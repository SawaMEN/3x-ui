import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import {
  ProxyBundleImportResultSchema,
  ProxyBundleSchema,
  ProxyPresetAssignmentSchema,
  ProxyPresetAssignmentsSchema,
  ProxyPresetInputSchema,
  ProxyPresetListSchema,
  ProxyPresetViewSchema,
  type ProxyBundle,
  type ProxyBundleImportResult,
  type ProxyPresetAssignment,
  type ProxyPresetInput,
  type ProxyPresetView,
} from '@/schemas/api/proxyPreset';
import { HttpUtil, type Msg } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import type { z } from 'zod';

const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } };

function requireParsedObj<T extends z.ZodType>(msg: Msg<unknown>, schema: T, context: string): z.infer<T> {
  const parsed = parseMsg(msg, schema, context, { strict: true }).obj;
  if (parsed == null) throw new Error(`${context} returned no object`);
  return parsed;
}

async function fetchPresets(): Promise<ProxyPresetView[]> {
  const msg = await HttpUtil.get('/panel/api/hosts/presets/list', undefined, { silent: true });
  if (!msg.success) throw new Error(msg.msg || 'Failed to fetch proxy presets');
  return requireParsedObj(msg, ProxyPresetListSchema, 'hosts/presets/list');
}

export function useProxyPresetsQuery() {
  return useQuery({
    queryKey: keys.proxyPresets.list(),
    queryFn: fetchPresets,
  });
}

export function useProxyPresetAssignmentsQuery() {
  return useQuery({
    queryKey: keys.proxyPresets.assignments(),
    queryFn: async (): Promise<ProxyPresetAssignment[]> => {
      const msg = await HttpUtil.get('/panel/api/hosts/presets/assignments', undefined, { silent: true });
      if (!msg.success) throw new Error(msg.msg || 'Failed to fetch proxy preset assignments');
      return requireParsedObj(msg, ProxyPresetAssignmentsSchema, 'hosts/presets/assignments');
    },
  });
}

export function useProxyPresetAssignment(groupId: string | null) {
  return useQuery({
    queryKey: keys.proxyPresets.assignment(groupId ?? ''),
    enabled: Boolean(groupId),
    queryFn: async (): Promise<ProxyPresetAssignment | null> => {
      const msg = await HttpUtil.get(
        `/panel/api/hosts/presets/assignment/${encodeURIComponent(groupId ?? '')}`,
        undefined,
        { silent: true },
      );
      if (!msg.success) throw new Error(msg.msg || 'Failed to fetch proxy preset assignment');
      if (msg.obj == null) return null;
      return requireParsedObj(msg, ProxyPresetAssignmentSchema, 'hosts/presets/assignment');
    },
  });
}

export function useProxyPresetMutations() {
  const queryClient = useQueryClient();
  const invalidate = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: keys.proxyPresets.root() }),
      queryClient.invalidateQueries({ queryKey: keys.hosts.root() }),
    ]);
  };

  const saveMutation = useMutation({
    mutationFn: async (input: ProxyPresetInput): Promise<ProxyPresetView> => {
      const payload = ProxyPresetInputSchema.parse(input);
      const msg = await HttpUtil.post('/panel/api/hosts/presets/save', payload, JSON_HEADERS);
      if (!msg.success) throw new Error(msg.msg || 'Failed to save proxy preset');
      return requireParsedObj(msg, ProxyPresetViewSchema, 'hosts/presets/save');
    },
    onSuccess: invalidate,
  });

  const updateMutation = useMutation({
    mutationFn: async ({ id, input }: { id: number; input: ProxyPresetInput }): Promise<ProxyPresetView> => {
      const payload = ProxyPresetInputSchema.parse(input);
      const msg = await HttpUtil.post(`/panel/api/hosts/presets/update/${id}`, payload, JSON_HEADERS);
      if (!msg.success) throw new Error(msg.msg || 'Failed to update proxy preset');
      return requireParsedObj(msg, ProxyPresetViewSchema, 'hosts/presets/update');
    },
    onSuccess: invalidate,
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      const msg = await HttpUtil.post(`/panel/api/hosts/presets/del/${id}`);
      if (!msg.success) throw new Error(msg.msg || 'Failed to delete proxy preset');
      return msg;
    },
    onSuccess: invalidate,
  });

  const assignMutation = useMutation({
    mutationFn: async ({ groupId, presetId }: { groupId: string; presetId: number }) => {
      const msg = await HttpUtil.post(
        `/panel/api/hosts/presets/assign/${encodeURIComponent(groupId)}`,
        { presetId },
        JSON_HEADERS,
      );
      if (!msg.success) throw new Error(msg.msg || 'Failed to assign proxy preset');
      return requireParsedObj(msg, ProxyPresetAssignmentSchema, 'hosts/presets/assign');
    },
    onSuccess: invalidate,
  });

  const assignManyMutation = useMutation({
    mutationFn: async ({ groupIds, presetId }: { groupIds: string[]; presetId: number | null }) => {
      const msg = await HttpUtil.post(
        '/panel/api/hosts/presets/assign/bulk',
        { groupIds, presetId },
        JSON_HEADERS,
      );
      if (!msg.success) throw new Error(msg.msg || 'Failed to assign proxy presets');
      return requireParsedObj(msg, ProxyPresetAssignmentsSchema, 'hosts/presets/assign/bulk');
    },
    onSuccess: invalidate,
  });

  const unassignMutation = useMutation({
    mutationFn: async (groupId: string) => {
      const msg = await HttpUtil.post(
        `/panel/api/hosts/presets/unassign/${encodeURIComponent(groupId)}`,
      );
      if (!msg.success) throw new Error(msg.msg || 'Failed to unassign proxy preset');
      return msg;
    },
    onSuccess: invalidate,
  });

  const importMutation = useMutation({
    mutationFn: async ({
      bundle,
      dryRun,
      allowMissingInbounds,
    }: {
      bundle: ProxyBundle;
      dryRun: boolean;
      allowMissingInbounds: boolean;
    }): Promise<ProxyBundleImportResult> => {
      const validatedBundle = ProxyBundleSchema.parse(bundle);
      const msg = await HttpUtil.post(
        '/panel/api/hosts/presets/bundle/import',
        { bundle: validatedBundle, dryRun, allowMissingInbounds },
        JSON_HEADERS,
      );
      if (!msg.success) throw new Error(msg.msg || 'Failed to import proxy bundle');
      return requireParsedObj(msg, ProxyBundleImportResultSchema, 'hosts/presets/bundle/import');
    },
    onSuccess: async (_, variables) => {
      if (!variables.dryRun) await invalidate();
    },
  });

  const exportBundle = async (): Promise<ProxyBundle> => {
    const msg = await HttpUtil.get('/panel/api/hosts/presets/bundle', undefined, { silent: true });
    if (!msg.success) throw new Error(msg.msg || 'Failed to export proxy bundle');
    return requireParsedObj(msg, ProxyBundleSchema, 'hosts/presets/bundle');
  };

  return {
    save: (input: ProxyPresetInput) => saveMutation.mutateAsync(input),
    update: (id: number, input: ProxyPresetInput) => updateMutation.mutateAsync({ id, input }),
    remove: (id: number) => deleteMutation.mutateAsync(id),
    assign: (groupId: string, presetId: number) => assignMutation.mutateAsync({ groupId, presetId }),
    assignMany: (groupIds: string[], presetId: number | null) =>
      assignManyMutation.mutateAsync({ groupIds, presetId }),
    unassign: (groupId: string) => unassignMutation.mutateAsync(groupId),
    importBundle: (bundle: ProxyBundle, dryRun: boolean, allowMissingInbounds: boolean) =>
      importMutation.mutateAsync({ bundle, dryRun, allowMissingInbounds }),
    exportBundle,
    saving: saveMutation.isPending || updateMutation.isPending,
    importing: importMutation.isPending,
    assigning: assignMutation.isPending || assignManyMutation.isPending || unassignMutation.isPending,
  };
}
