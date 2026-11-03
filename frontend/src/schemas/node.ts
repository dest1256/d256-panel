import { z } from 'zod';

export const NodeRecordSchema = z
  .object({
    id: z.number(),
    name: z.string().optional(),
    remark: z.string().optional(),
    scheme: z.string().optional(),
    address: z.string().optional(),
    port: z.number().optional(),
    basePath: z.string().optional(),
    apiToken: z.string().optional(),
    hasApiToken: z.boolean().optional(),
    enable: z.boolean().optional(),
    status: z.string().optional(),
    latencyMs: z.number().optional(),
    cpuPct: z.number().optional(),
    memPct: z.number().optional(),
    xrayVersion: z.string().optional(),
    panelVersion: z.string().optional(),
    uptimeSecs: z.number().optional(),
    unavailableMetrics: z.array(z.string()).optional(),
    inboundCount: z.number().optional(),
    clientCount: z.number().optional(),
    onlineCount: z.number().optional(),
    activeCount: z.number().optional(),
    disabledCount: z.number().optional(),
    depletedCount: z.number().optional(),
    lastHeartbeat: z.number().optional(),
    lastError: z.string().optional(),
    // Xray state captured from the remote node's own /panel/api/server/status.
    // Lets the nodes list show a distinct indicator when the panel API is reachable
    // (status=online) but the Xray core on that node has failed.
    xrayState: z.string().optional(),
    xrayError: z.string().optional(),
    allowPrivateAddress: z.boolean().optional(),
    tlsVerifyMode: z.enum(['verify', 'skip', 'pin', 'mtls']).optional(),
    pinnedCertSha256: z.string().optional(),
    inboundSyncMode: z.enum(['all', 'selected']).optional(),
    // Backend serializes a nil []string as null for nodes saved before #5178.
    inboundTags: z.array(z.string()).nullish(),
    outboundTag: z.string().optional(),
    // Multi-hop node tree (#4983): a node's stable GUID, its parent's GUID, and
    // whether it's a read-only transitive sub-node surfaced from a downstream node.
    guid: z.string().optional(),
    parentGuid: z.string().optional(),
    transitive: z.boolean().optional(),
  })
  .loose();

export const NodeListSchema = z.array(NodeRecordSchema);

export type NodeRecord = z.infer<typeof NodeRecordSchema>;
