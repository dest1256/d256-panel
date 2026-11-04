import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';

import { HttpUtil } from '@/utils';
import { keys } from '@/api/queryKeys';

// Mirrors the Go NodeLive struct (internal/web/controller/node.go).
export interface NodeLive {
  nodeId: number;
  online: boolean;
  latencyMs: number;
  lastSeen: number;
  nodeVersion: string;
  xrayVersion: string;
  xrayState: string;
  xrayError: string;
  cpuPct: number;
  cpuCores: number;
  cpuMhz: number;
  memTotal: number;
  memUsed: number;
  swapTotal: number;
  swapUsed: number;
  diskTotal: number;
  diskUsed: number;
  netUpBps: number;
  netDownBps: number;
  netSentTotal: number;
  netRecvTotal: number;
  tcpSockets: number;
  udpSockets: number;
  uptimeSecs: number;
  journalBytes: number;
  onlineClients: number;
  // The last self-update asked of the node: '', 'requested', 'ok' or 'failed'.
  upgradeState: string;
  upgradeVersion: string;
  upgradeError: string;
}

const LIVE_REFRESH_MS = 5000;

async function fetchLive(): Promise<NodeLive[]> {
  const msg = await HttpUtil.get<NodeLive[]>('/panel/api/nodes/live', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch node metrics');
  return Array.isArray(msg.obj) ? msg.obj : [];
}

export function useNodesLiveQuery() {
  const query = useQuery({
    queryKey: keys.nodes.live(),
    queryFn: fetchLive,
    refetchInterval: LIVE_REFRESH_MS,
  });
  const byId = useMemo(() => {
    const m = new Map<number, NodeLive>();
    for (const l of query.data ?? []) m.set(l.nodeId, l);
    return m;
  }, [query.data]);
  return { live: query.data ?? [], byId };
}
