import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge, Button, Card, Descriptions, Dropdown, Switch, Table, Tag, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  DeleteOutlined,
  EditOutlined,
  EyeInvisibleOutlined,
  EyeOutlined,
  KeyOutlined,
  MoreOutlined,
  PlusOutlined,
  SyncOutlined,
  CloudSyncOutlined,
} from '@ant-design/icons';

import NodeHistoryPanel from './NodeHistoryPanel';
import type { NodeRecord } from '@/api/queries/useNodesQuery';
import type { NodeLive } from '@/api/queries/useNodesLiveQuery';
import { CPUFormatter, SizeFormatter } from '@/utils';
import { isNewerRelease } from '@/lib/nodes/version';
import { activateOnKey } from '@/utils/a11y';
import './NodeList.css';

interface NodeListProps {
  nodes: NodeRecord[];
  liveById: Map<number, NodeLive>;
  // Newest published node release ('' when unknown).
  latestVersion?: string;
  loading?: boolean;
  onAdd: () => void;
  onEdit: (node: NodeRecord) => void;
  onDelete: (node: NodeRecord) => void;
  onNewToken: (node: NodeRecord) => void;
  onUpgrade: (node: NodeRecord) => void;
  onToggleEnable: (node: NodeRecord, next: boolean) => void;
}

function badgeStatus(status?: string): 'success' | 'error' | 'warning' | 'default' {
  switch (status) {
    case 'online':
      return 'success';
    case 'offline':
      return 'error';
    case 'pending':
      return 'warning';
    default:
      return 'default';
  }
}

function formatUptime(secs?: number): string {
  if (!secs) return '-';
  const days = Math.floor(secs / 86400);
  const hours = Math.floor((secs % 86400) / 3600);
  if (days > 0) return `${days}d ${hours}h`;
  const mins = Math.floor((secs % 3600) / 60);
  return hours > 0 ? `${hours}h ${mins}m` : `${mins}m`;
}

function usage(used: number, total: number): string {
  if (!total) return '-';
  return `${SizeFormatter.sizeFormat(used)} / ${SizeFormatter.sizeFormat(total)} (${((used / total) * 100).toFixed(0)}%)`;
}

function LiveDetails({ live }: { live?: NodeLive }) {
  const { t } = useTranslation();
  if (!live?.online && !live?.cpuCores) {
    return <div style={{ opacity: 0.65 }}>{t('pages.nodes.noLiveData')}</div>;
  }
  return (
    <Descriptions size="small" column={{ xs: 1, sm: 2, lg: 3 }} bordered>
      <Descriptions.Item label={t('pages.nodes.cpuInfo')}>
        {CPUFormatter.cpuCoreFormat(live.cpuCores)} · {CPUFormatter.cpuSpeedFormat(live.cpuMhz)} ·{' '}
        {live.cpuPct.toFixed(1)}%
      </Descriptions.Item>
      <Descriptions.Item label={t('pages.nodes.ram')}>
        {usage(live.memUsed, live.memTotal)}
      </Descriptions.Item>
      <Descriptions.Item label={t('pages.nodes.swap')}>
        {usage(live.swapUsed, live.swapTotal)}
      </Descriptions.Item>
      <Descriptions.Item label={t('pages.nodes.disk')}>
        {usage(live.diskUsed, live.diskTotal)}
      </Descriptions.Item>
      <Descriptions.Item label={t('pages.nodes.netSpeed')}>
        ↑ {SizeFormatter.speedFormat(live.netUpBps)} · ↓{' '}
        {SizeFormatter.speedFormat(live.netDownBps)}
      </Descriptions.Item>
      <Descriptions.Item label={t('pages.nodes.netTotal')}>
        ↑ {SizeFormatter.sizeFormat(live.netSentTotal)} · ↓{' '}
        {SizeFormatter.sizeFormat(live.netRecvTotal)}
      </Descriptions.Item>
      <Descriptions.Item label={t('pages.nodes.sockets')}>
        TCP {live.tcpSockets} · UDP {live.udpSockets}
      </Descriptions.Item>
      <Descriptions.Item label={t('pages.nodes.onlineClients')}>
        {live.onlineClients}
      </Descriptions.Item>
      <Descriptions.Item label={t('pages.nodes.journal')}>
        {SizeFormatter.sizeFormat(live.journalBytes)}
      </Descriptions.Item>
      <Descriptions.Item label={t('pages.nodes.nodeVersion')}>
        {live.nodeVersion || '-'}
      </Descriptions.Item>
      <Descriptions.Item label={t('pages.nodes.xrayVersion')}>
        {live.xrayVersion || '-'}
      </Descriptions.Item>
      <Descriptions.Item label={t('pages.nodes.xrayState')}>
        {live.xrayState
          ? t(`pages.nodes.xrayStates.${live.xrayState}`, { defaultValue: live.xrayState })
          : '-'}
        {live.xrayError ? ` — ${live.xrayError}` : ''}
      </Descriptions.Item>
    </Descriptions>
  );
}

export default function NodeList({
  nodes,
  liveById,
  loading = false,
  onAdd,
  onEdit,
  onDelete,
  onNewToken,
  onUpgrade,
  latestVersion,
  onToggleEnable,
}: NodeListProps) {
  const { t } = useTranslation();
  const [showAddress, setShowAddress] = useState(false);

  const columns = useMemo<ColumnsType<NodeRecord>>(
    () => [
      {
        title: t('pages.nodes.status'),
        dataIndex: 'status',
        width: 130,
        render: (status: string, n) =>
          status === 'online' ? (
            <span>
              <span className="online-dot" /> {t('pages.nodes.statusValues.online')}
            </span>
          ) : (
            <Badge
              status={badgeStatus(status)}
              text={t(`pages.nodes.statusValues.${status || 'unknown'}`)}
              title={n.lastError || undefined}
            />
          ),
      },
      {
        title: t('pages.nodes.name'),
        dataIndex: 'name',
        render: (_: string, n) => (
          <div className="name-cell">
            <span className="name">{n.name}</span>
            {n.remark ? <span className="remark">{n.remark}</span> : null}
          </div>
        ),
      },
      {
        title: (
          <span className="address-header">
            {t('pages.nodes.address')}
            <span
              role="button"
              tabIndex={0}
              aria-label={t('pages.nodes.address')}
              aria-pressed={showAddress}
              className="ip-toggle-icon"
              onClick={() => setShowAddress((v) => !v)}
              onKeyDown={activateOnKey(() => setShowAddress((v) => !v))}
            >
              {showAddress ? <EyeOutlined aria-hidden /> : <EyeInvisibleOutlined aria-hidden />}
            </span>
          </span>
        ),
        dataIndex: 'address',
        render: (a: string) => (
          <span className={showAddress ? 'address-visible' : 'address-hidden'}>{a}</span>
        ),
      },
      {
        title: t('pages.nodes.latency'),
        width: 100,
        render: (_: unknown, n) => {
          const ms = liveById.get(n.id)?.latencyMs ?? n.latencyMs;
          return n.status === 'online' ? `${ms ?? 0} ms` : '-';
        },
      },
      {
        title: t('pages.nodes.cpu'),
        width: 90,
        render: (_: unknown, n) => (n.status === 'online' ? `${(n.cpuPct ?? 0).toFixed(1)}%` : '-'),
      },
      {
        title: t('pages.nodes.mem'),
        width: 90,
        render: (_: unknown, n) => (n.status === 'online' ? `${(n.memPct ?? 0).toFixed(1)}%` : '-'),
      },
      {
        title: t('pages.nodes.uptime'),
        width: 100,
        render: (_: unknown, n) => (n.status === 'online' ? formatUptime(n.uptimeSecs) : '-'),
      },
      {
        title: t('pages.nodes.nodeVersion'),
        width: 170,
        render: (_: unknown, n) => {
          const live = liveById.get(n.id ?? 0);
          const state = live?.upgradeState;
          const updating =
            state === 'requested' || (state === 'ok' && live?.nodeVersion !== live?.upgradeVersion);
          return (
            <span>
              {n.panelVersion ? <Tag>{n.panelVersion}</Tag> : '-'}
              {updating && (
                <Tag icon={<SyncOutlined spin aria-hidden />} color="processing">
                  {t('pages.nodes.updating')}
                </Tag>
              )}
              {state === 'failed' && (
                <Tooltip title={live?.upgradeError}>
                  <Tag color="error">{t('pages.nodes.updateFailed')}</Tag>
                </Tooltip>
              )}
              {!updating &&
                n.status === 'online' &&
                isNewerRelease(latestVersion, n.panelVersion) && (
                  <Tooltip title={t('pages.nodes.updateAvailableHint')}>
                    <Tag color="gold">
                      {t('pages.nodes.updateAvailable', { version: latestVersion })}
                    </Tag>
                  </Tooltip>
                )}
            </span>
          );
        },
      },
      {
        title: t('pages.nodes.enable'),
        dataIndex: 'enable',
        width: 90,
        render: (enable: boolean, n) => (
          <Switch
            size="small"
            checked={!!enable}
            aria-label={t('pages.nodes.enable')}
            onChange={(v) => onToggleEnable(n, v)}
          />
        ),
      },
      {
        title: t('pages.nodes.actions'),
        width: 70,
        render: (_: unknown, n) => (
          <Dropdown
            trigger={['click']}
            menu={{
              items: [
                {
                  key: 'edit',
                  icon: <EditOutlined aria-hidden />,
                  label: t('edit'),
                  onClick: () => onEdit(n),
                },
                {
                  key: 'upgrade',
                  icon: <CloudSyncOutlined aria-hidden />,
                  label: t('pages.nodes.upgrade'),
                  disabled: n.status !== 'online',
                  onClick: () => onUpgrade(n),
                },
                {
                  key: 'token',
                  icon: <KeyOutlined aria-hidden />,
                  label: t('pages.nodes.newToken'),
                  onClick: () => onNewToken(n),
                },
                {
                  key: 'del',
                  icon: <DeleteOutlined aria-hidden />,
                  danger: true,
                  label: t('delete'),
                  onClick: () => onDelete(n),
                },
              ],
            }}
          >
            <Button
              type="text"
              aria-label={t('pages.nodes.actions')}
              icon={<MoreOutlined aria-hidden />}
            />
          </Dropdown>
        ),
      },
    ],
    [
      t,
      showAddress,
      liveById,
      latestVersion,
      onEdit,
      onDelete,
      onNewToken,
      onUpgrade,
      onToggleEnable,
    ],
  );

  return (
    <Card size="small">
      <div className="toolbar">
        <Button type="primary" icon={<PlusOutlined aria-hidden />} onClick={onAdd}>
          {t('pages.nodes.addNode')}
        </Button>
      </div>
      <Table<NodeRecord>
        rowKey="id"
        size="small"
        loading={loading}
        columns={columns}
        dataSource={nodes}
        pagination={false}
        scroll={{ x: 'max-content' }}
        expandable={{
          expandedRowRender: (n) => (
            <div>
              <LiveDetails live={liveById.get(n.id)} />
              <NodeHistoryPanel node={{ id: n.id }} />
            </div>
          ),
          rowExpandable: (n) => n.status !== 'pending',
        }}
      />
    </Card>
  );
}
