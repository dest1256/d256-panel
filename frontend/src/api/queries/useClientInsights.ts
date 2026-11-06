import { useQuery } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';

// Mirror the Go views in internal/web/controller/client_insights.go and internal/nodelink/insights.go.
export interface ActiveConnection {
  nodeId: number;
  node: string;
  inbound: string;
  sourceIp: string;
  dest: string;
  port: number;
  network: string;
  start: number;
  up: number;
  down: number;
}

export interface ClientDevice {
  nodeId: number;
  node: string;
  ip: string;
  lastSeen: number;
}

// The part of a destination's usage attributed to one device. hwid is "" when the device is unknown.
export interface DestinationDevice {
  hwid: string;
  short: string;
  label: string;
  up: number;
  down: number;
  lastSeen: number;
}

export interface DestinationRow {
  dest: string;
  network: string;
  nodeId: number;
  node: string;
  up: number;
  down: number;
  conns: number;
  total: number;
  // Unix seconds of the latest use.
  lastSeen: number;
  devices: DestinationDevice[];
}

// A device (HWID) that fetched the client's subscription.
export interface ClientHwidDevice {
  hwid: string;
  short: string;
  label: string;
  firstSeen: number;
  lastSeen: number;
  lastIp: string;
}

// Value of the device filter that selects usage not attributed to any device.
export const NO_DEVICE = '-';

export interface TrafficPoint {
  t: number;
  up: number;
  down: number;
}

export interface TrafficSeries {
  name: string;
  nodeId: number;
  points: TrafficPoint[];
}

export interface ClientTraffic {
  from: number;
  to: number;
  step: number;
  total: TrafficPoint;
  nodes: TrafficSeries[];
  inbounds: TrafficSeries[];
}

export type InsightsPeriod = '1h' | '24h' | '7d' | '30d' | 'custom';

export interface InsightsRange {
  period: InsightsPeriod;
  // Unix seconds, only used by the custom period.
  from?: number;
  to?: number;
}

export const LIVE_REFRESH_MS = 3000;

const BASE = '/panel/api/clientInsights';

async function fetchObj<T>(path: string, params: Record<string, unknown>, fallback: T): Promise<T> {
  const msg = await HttpUtil.get<T>(`${BASE}/${path}`, params, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Request failed');
  return (msg.obj ?? fallback) as T;
}

function rangeParams(r: InsightsRange): Record<string, unknown> {
  return r.period === 'custom'
    ? { period: 'custom', from: r.from, to: r.to }
    : { period: r.period };
}

const live = (enabled: boolean) => ({
  enabled,
  refetchInterval: enabled ? LIVE_REFRESH_MS : (false as const),
});

export function useActiveConnections(email: string, enabled: boolean) {
  return useQuery({
    queryKey: ['clientInsights', 'connections', email],
    queryFn: () => fetchObj<ActiveConnection[]>('connections', { email }, []),
    ...live(enabled && !!email),
  });
}

export function useClientDevices(email: string, enabled: boolean) {
  return useQuery({
    queryKey: ['clientInsights', 'devices', email],
    queryFn: () => fetchObj<ClientDevice[]>('devices', { email }, []),
    ...live(enabled && !!email),
  });
}

export function useClientTraffic(email: string, range: InsightsRange, enabled: boolean) {
  return useQuery({
    queryKey: ['clientInsights', 'traffic', email, range],
    queryFn: () =>
      fetchObj<ClientTraffic | null>('traffic', { email, ...rangeParams(range) }, null),
    enabled: enabled && !!email && (range.period !== 'custom' || !!(range.from && range.to)),
  });
}

export interface DestinationsFilter {
  nodeId?: number;
  q?: string;
  hwid?: string;
  sort: 'dest' | 'up' | 'down' | 'total' | 'conns' | 'last';
  order: 'asc' | 'desc';
}

export function useClientDestinations(
  email: string,
  range: InsightsRange,
  filter: DestinationsFilter,
  enabled: boolean,
) {
  return useQuery({
    queryKey: ['clientInsights', 'destinations', email, range, filter],
    queryFn: () =>
      fetchObj<DestinationRow[]>(
        'destinations',
        {
          email,
          ...rangeParams(range),
          nodeId: filter.nodeId || undefined,
          hwid: filter.hwid || undefined,
          q: filter.q || undefined,
          sort: filter.sort,
          order: filter.order,
        },
        [],
      ),
    enabled: enabled && !!email && (range.period !== 'custom' || !!(range.from && range.to)),
  });
}

export function useClientHwidDevices(email: string, enabled: boolean) {
  return useQuery({
    queryKey: ['clientInsights', 'hwids', email],
    queryFn: () => fetchObj<ClientHwidDevice[]>('hwids', { email }, []),
    enabled: enabled && !!email,
  });
}
