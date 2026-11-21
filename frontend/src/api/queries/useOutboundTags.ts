import { useQuery } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { useAllSettings } from '@/api/queries/useAllSettings';
import { fetchXrayConfig } from '@/hooks/useXraySetting';
import { isOutboundProtocol } from '@/schemas/primitives';
import { HttpUtil } from '@/utils';

// Available outbound (and balancer-eligible) tags the user can route an mtproto
// inbound's Telegram traffic to. Shares the cached xray config query so opening
// the inbound form costs no extra request when the Xray page was already
// visited; `select` derives just the tag list without disturbing other readers.
export function useOutboundTags(opts?: { excludeBlackhole?: boolean; enabled?: boolean }) {
  const excludeBlackhole = opts?.excludeBlackhole ?? false;
  return useQuery({
    queryKey: keys.xray.config(),
    queryFn: fetchXrayConfig,
    enabled: opts?.enabled,
    staleTime: Infinity,
    select: (data): string[] => {
      const tags = new Set<string>();
      for (const o of data?.xraySetting?.outbounds ?? []) {
        const ob = o as { tag?: string; protocol?: string } | null;
        if (!ob?.tag) continue;
        if (excludeBlackhole && isOutboundProtocol(ob, 'blackhole')) continue;
        tags.add(ob.tag);
      }
      for (const t of data?.subscriptionOutboundTags ?? []) {
        if (t) tags.add(t);
      }
      // Balancers are valid routing targets too — injectMtprotoEgress emits a
      // balancerTag rule when the chosen tag names a balancer.
      const balancers = (
        data?.xraySetting?.routing as { balancers?: Array<{ tag?: string }> } | undefined
      )?.balancers;
      for (const b of balancers ?? []) {
        if (b?.tag) tags.add(b.tag);
      }
      return [...tags];
    },
  });
}

interface NativeRoutingSnapshot {
  config: {
    outbounds?: { tag?: string; type?: string }[];
    endpoints?: { tag?: string }[];
  };
  subscriptionTags?: string[];
}

export function useTelemtOutboundTags(enabled: boolean, nodeId?: number | null) {
  const { allSetting, fetched } = useAllSettings();
  const native = nodeId == null && allSetting.coreType !== 'xray';
  const xray = useOutboundTags({ excludeBlackhole: true, enabled: enabled && fetched && !native });
  const singBox = useQuery({
    queryKey: ['sing-box', 'telemt-routing-targets', allSetting.coreType],
    enabled: enabled && fetched && native,
    queryFn: async () => {
      const msg = await HttpUtil.get<NativeRoutingSnapshot>(
        '/panel/api/setting/singbox/config',
        undefined,
        { silent: true },
      );
      if (!msg.success || !msg.obj?.config) {
        throw new Error(msg.msg || 'Failed to load Sing-box routing targets');
      }
      const { config, subscriptionTags } = msg.obj;
      const tags = [
        ...(config.outbounds ?? []).filter((row) => row.type !== 'block').map((row) => row.tag),
        ...(config.endpoints ?? []).map((row) => row.tag),
        ...(subscriptionTags ?? []),
      ];
      return [...new Set(tags.filter((tag): tag is string => Boolean(tag)))];
    },
  });
  return native ? singBox : xray;
}

export interface OutboundTagGroups {
  outbounds: string[];
  balancers: string[];
}

// Same data as useOutboundTags, but keeps outbound and balancer tags apart so a
// picker can render them in labeled groups (like the panel-outbound selector)
// instead of one flat list.
export function useOutboundTagGroups(opts?: { excludeBlackhole?: boolean }) {
  const excludeBlackhole = opts?.excludeBlackhole ?? false;
  return useQuery({
    queryKey: keys.xray.config(),
    queryFn: fetchXrayConfig,
    staleTime: Infinity,
    select: (data): OutboundTagGroups => {
      const outbounds = new Set<string>();
      for (const o of data?.xraySetting?.outbounds ?? []) {
        const ob = o as { tag?: string; protocol?: string } | null;
        if (!ob?.tag) continue;
        if (excludeBlackhole && isOutboundProtocol(ob, 'blackhole')) continue;
        outbounds.add(ob.tag);
      }
      for (const t of data?.subscriptionOutboundTags ?? []) {
        if (t) outbounds.add(t);
      }
      const balancers: string[] = [];
      const bal = (
        data?.xraySetting?.routing as { balancers?: Array<{ tag?: string }> } | undefined
      )?.balancers;
      for (const b of bal ?? []) {
        if (b?.tag && !outbounds.has(b.tag)) balancers.push(b.tag);
      }
      return { outbounds: [...outbounds], balancers };
    },
  });
}
