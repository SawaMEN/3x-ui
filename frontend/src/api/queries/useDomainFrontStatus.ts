import { useQuery } from '@tanstack/react-query';
import { keys } from '@/api/queryKeys';
import { DomainFrontStatusSchema } from '@/generated/zod';
import { HttpUtil } from '@/utils';

export function useDomainFrontStatus() {
  return useQuery({
    queryKey: keys.settings.domains(),
    queryFn: async () => {
      const msg = await HttpUtil.get('/panel/api/domains/status', undefined, { silent: true });
      if (!msg?.success) throw new Error(msg?.msg || 'Failed to load domain settings');
      return DomainFrontStatusSchema.parse(msg.obj);
    },
    refetchInterval: 60000,
    refetchIntervalInBackground: false,
    retry: false,
  });
}
