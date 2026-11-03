import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge, Card, Col, Empty, Progress, Row, Typography } from 'antd';

import NodeHistoryPanel from '../nodes/NodeHistoryPanel';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import type { NodeRecord } from '@/api/queries/useNodesQuery';
import { useNodesLiveQuery } from '@/api/queries/useNodesLiveQuery';
import type { NodeLive } from '@/api/queries/useNodesLiveQuery';
import { CPUFormatter, SizeFormatter } from '@/utils';
import { activateOnKey } from '@/utils/a11y';
import {
  USAGE_CRIT_COLOR,
  USAGE_CRIT_PERCENT,
  USAGE_WARN_COLOR,
  USAGE_WARN_PERCENT,
} from '@/models/status';

function usageColor(pct: number): string | undefined {
  if (pct >= USAGE_CRIT_PERCENT) return USAGE_CRIT_COLOR;
  if (pct >= USAGE_WARN_PERCENT) return USAGE_WARN_COLOR;
  return undefined;
}

function pctOf(used: number, total: number): number {
  return total > 0 ? Math.min(100, (used / total) * 100) : 0;
}

function formatUptime(secs: number): string {
  const days = Math.floor(secs / 86400);
  const hours = Math.floor((secs % 86400) / 3600);
  if (days > 0) return `${days}d ${hours}h`;
  return `${hours}h ${Math.floor((secs % 3600) / 60)}m`;
}

function Meter({ label, pct, detail }: { label: string; pct: number; detail?: string }) {
  return (
    <div className="ov-node-meter">
      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12 }}>
        <span>{label}</span>
        <span style={{ opacity: 0.7 }}>{detail ?? `${pct.toFixed(0)}%`}</span>
      </div>
      <Progress
        percent={Math.round(pct)}
        showInfo={false}
        size="small"
        strokeColor={usageColor(pct)}
      />
    </div>
  );
}

function NodeCard({
  node,
  live,
  expanded,
  onToggle,
}: {
  node: NodeRecord;
  live?: NodeLive;
  expanded: boolean;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  const online = node.status === 'online' && !!live;
  const badge = online ? 'success' : node.status === 'pending' ? 'warning' : 'error';
  return (
    <Card
      size="small"
      hoverable
      title={
        <span
          role="button"
          tabIndex={0}
          aria-expanded={expanded}
          onClick={onToggle}
          onKeyDown={activateOnKey(onToggle)}
        >
          <Badge status={badge} /> {node.name}
        </span>
      }
      extra={
        <span style={{ fontSize: 12, opacity: 0.7 }}>
          {online
            ? `${live.latencyMs} ms`
            : t(`pages.nodes.statusValues.${node.status || 'unknown'}`)}
        </span>
      }
    >
      {online ? (
        <>
          <Meter
            label={t('pages.index.cpu')}
            pct={live.cpuPct}
            detail={`${CPUFormatter.cpuCoreFormat(live.cpuCores)} · ${live.cpuPct.toFixed(0)}%`}
          />
          <Meter
            label={t('pages.nodes.ram')}
            pct={pctOf(live.memUsed, live.memTotal)}
            detail={`${SizeFormatter.sizeFormat(live.memUsed)} / ${SizeFormatter.sizeFormat(live.memTotal)}`}
          />
          <Meter
            label={t('pages.nodes.swap')}
            pct={pctOf(live.swapUsed, live.swapTotal)}
            detail={`${SizeFormatter.sizeFormat(live.swapUsed)} / ${SizeFormatter.sizeFormat(live.swapTotal)}`}
          />
          <Meter
            label={t('pages.nodes.disk')}
            pct={pctOf(live.diskUsed, live.diskTotal)}
            detail={`${SizeFormatter.sizeFormat(live.diskUsed)} / ${SizeFormatter.sizeFormat(live.diskTotal)}`}
          />
          <div style={{ fontSize: 12, marginTop: 6, lineHeight: 1.7 }}>
            <div>
              {t('pages.nodes.netSpeed')}: ↑ {SizeFormatter.speedFormat(live.netUpBps)} · ↓{' '}
              {SizeFormatter.speedFormat(live.netDownBps)}
            </div>
            <div>
              {t('pages.nodes.netTotal')}: ↑ {SizeFormatter.sizeFormat(live.netSentTotal)} · ↓{' '}
              {SizeFormatter.sizeFormat(live.netRecvTotal)}
            </div>
            <div>
              {t('pages.nodes.sockets')}: TCP {live.tcpSockets} · UDP {live.udpSockets}
            </div>
            <div>
              {t('pages.nodes.onlineClients')}: {live.onlineClients} · {t('pages.nodes.journal')}:{' '}
              {SizeFormatter.sizeFormat(live.journalBytes)}
            </div>
            <div>
              {t('pages.nodes.uptime')}: {formatUptime(live.uptimeSecs)} · {live.nodeVersion || '-'}
            </div>
          </div>
          {expanded ? <NodeHistoryPanel node={{ id: node.id }} /> : null}
        </>
      ) : (
        <div style={{ opacity: 0.65 }}>{t('pages.nodes.noLiveData')}</div>
      )}
    </Card>
  );
}

// Live view of every node: one card per node with its latest reported host metrics.
export default function NodesOverview() {
  const { t } = useTranslation();
  const { nodes, totals, fetched } = useNodesQuery();
  const { byId } = useNodesLiveQuery();
  const [expandedId, setExpandedId] = useState<number | null>(null);

  if (!fetched) return null;
  return (
    <section className="ov-nodes" aria-label={t('pages.nodes.title')}>
      <Typography.Title level={5} style={{ marginTop: 0 }}>
        {t('pages.nodes.title')}{' '}
        <span style={{ fontWeight: 400, opacity: 0.7 }}>
          {totals.online}/{totals.total}
        </span>
      </Typography.Title>
      {nodes.length === 0 ? (
        <Empty description={t('pages.nodes.noNodes')} />
      ) : (
        <Row gutter={[12, 12]}>
          {nodes.map((n) => (
            <Col key={n.id} xs={24} md={12} xl={8}>
              <NodeCard
                node={n}
                live={byId.get(n.id)}
                expanded={expandedId === n.id}
                onToggle={() => setExpandedId(expandedId === n.id ? null : n.id)}
              />
            </Col>
          ))}
        </Row>
      )}
    </section>
  );
}
