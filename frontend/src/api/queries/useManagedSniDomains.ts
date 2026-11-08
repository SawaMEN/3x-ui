import { useQuery } from '@tanstack/react-query';
import { HttpUtil } from '@/utils';

export interface ManagedSniDomain {
  name: string;
  source: 'telemt' | 'gateway' | 'remote';
  certFile: string;
  keyFile: string;
  nodeId?: number;
  nodeName?: string;
}

// Certificate paths belong to the hosting node and cannot be reused on
// other nodes, even when those nodes share a public SNI hostname.
export function canApplyManagedSniCertificate(
  domain: ManagedSniDomain,
  nodeId: number | null | undefined,
): boolean {
  if (!domain.certFile || !domain.keyFile) return false;
  if (domain.source === 'remote') {
    return nodeId != null && domain.nodeId != null && nodeId === domain.nodeId;
  }
  return nodeId == null;
}

export function useManagedSniDomains() {
  return useQuery({
    queryKey: ['settings', 'managed-sni'],
    queryFn: async (): Promise<ManagedSniDomain[]> => {
      const result = await HttpUtil.get('/panel/api/domains/sni', undefined, { silent: true });
      if (!result?.success || !Array.isArray(result.obj))
        throw new Error(result?.msg || 'Failed to load managed SNI');
      return (result.obj as ManagedSniDomain[]).filter(
        (domain) =>
          typeof domain.name === 'string' &&
          typeof domain.source === 'string' &&
          typeof domain.certFile === 'string' &&
          typeof domain.keyFile === 'string',
      );
    },
    refetchInterval: 60000,
    refetchIntervalInBackground: false,
    retry: false,
  });
}
