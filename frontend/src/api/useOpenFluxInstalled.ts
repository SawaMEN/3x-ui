import { useQuery } from '@tanstack/react-query';
import { HttpUtil } from '@/utils';

// Status reads never install or download a component, including on a remote node.
export function useOpenFluxInstalled(open: boolean, nodeId = 0): boolean {
  const status = useQuery({
    queryKey: ['openflux-installed', nodeId],
    enabled: open,
    staleTime: 0,
    retry: false,
    refetchOnMount: 'always',
    queryFn: async () => {
      const suffix = nodeId > 0 ? `?nodeId=${nodeId}` : '';
      const result = await HttpUtil.get<{ installed?: boolean }>(
        `/panel/api/server/openflux/status${suffix}`,
        undefined,
        { silent: true },
      );
      return result.success === true && result.obj?.installed === true;
    },
  });
  return open && !status.isFetching && status.isSuccess && status.data === true;
}
