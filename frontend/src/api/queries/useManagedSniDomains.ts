import { useQuery } from '@tanstack/react-query';
import { HttpUtil } from '@/utils';

export interface ManagedSniDomain {
  name: string;
  source: 'telemt' | 'gateway';
  certFile: string;
  keyFile: string;
}

// Always request the authenticated main panel, even while editing a remote
// node. Local file paths are deliberately not portable to a remote node.
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
