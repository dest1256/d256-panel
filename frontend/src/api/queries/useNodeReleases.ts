import { useQuery } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';

// Mirrors the Go NodeReleases struct (internal/web/controller/node.go).
export interface NodeReleases {
  latest: string;
  repo: string;
  checkedAt: number;
  error: string;
}

const RELEASES_REFRESH_MS = 30 * 60 * 1000;

export function useNodeReleases() {
  const query = useQuery({
    queryKey: ['nodes', 'releases'],
    queryFn: async (): Promise<NodeReleases> => {
      const msg = await HttpUtil.get<NodeReleases>('/panel/api/nodes/releases', undefined, {
        silent: true,
      });
      if (!msg?.success || !msg.obj) throw new Error(msg?.msg || 'Failed to fetch releases');
      return msg.obj;
    },
    staleTime: RELEASES_REFRESH_MS,
    refetchInterval: RELEASES_REFRESH_MS,
  });
  return { releases: query.data, refresh: query.refetch };
}
