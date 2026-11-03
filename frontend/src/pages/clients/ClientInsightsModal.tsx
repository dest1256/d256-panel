import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  DatePicker,
  Empty,
  Input,
  Modal,
  Segmented,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
} from 'antd';
import type { TableColumnsType } from 'antd';
import { DownloadOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';

import { FileManager, SizeFormatter } from '@/utils';
import Sparkline from '@/components/viz/Sparkline';
import {
  useActiveConnections,
  useClientDestinations,
  useClientDevices,
  useClientTraffic,
  type ActiveConnection,
  type ClientDevice,
  type DestinationRow,
  type DestinationsFilter,
  type InsightsPeriod,
  type InsightsRange,
  type TrafficSeries,
} from '@/api/queries/useClientInsights';
import { destinationsToCsv, fillSeries } from '@/lib/clients/insights';

interface ClientInsightsModalProps {
  open: boolean;
  email: string;
  onClose: () => void;
}

const PERIODS: InsightsPeriod[] = ['1h', '24h', '7d', '30d', 'custom'];

function timeLabel(unix: number, step: number): string {
  const d = dayjs.unix(unix);
  return step >= 3600 ? d.format('MM-DD HH:mm') : d.format('HH:mm');
}

function sizeSorter<T>(key: keyof T) {
  return (a: T, b: T) => Number(a[key]) - Number(b[key]);
}

export default function ClientInsightsModal({ open, email, onClose }: ClientInsightsModalProps) {
  const { t } = useTranslation();
  const [tab, setTab] = useState('traffic');
  const [period, setPeriod] = useState<InsightsPeriod>('24h');
  const [custom, setCustom] = useState<[number, number] | null>(null);
  const [split, setSplit] = useState<'node' | 'inbound'>('node');
  const [search, setSearch] = useState('');
  const [nodeId, setNodeId] = useState<number | undefined>(undefined);
  const [sort, setSort] = useState<DestinationsFilter['sort']>('total');
  const [order, setOrder] = useState<DestinationsFilter['order']>('desc');

  const range = useMemo<InsightsRange>(
    () => (period === 'custom' ? { period, from: custom?.[0], to: custom?.[1] } : { period }),
    [period, custom],
  );

  const traffic = useClientTraffic(email, range, open && tab === 'traffic');
  const active = useActiveConnections(email, open && tab === 'active');
  const devices = useClientDevices(email, open && tab === 'devices');
  const dests = useClientDestinations(
    email,
    range,
    { nodeId, q: search, sort, order },
    open && tab === 'destinations',
  );
  // The node filter lists every node the client has traffic on.
  const nodeOptions = useMemo(() => {
    const m = new Map<number, string>();
    for (const n of traffic.data?.nodes ?? []) m.set(n.nodeId, n.name);
    for (const r of dests.data ?? []) m.set(r.nodeId, r.node);
    return [...m].map(([value, label]) => ({ value, label: label || `#${value}` }));
  }, [traffic.data, dests.data]);

  const periodBar = (
    <Space wrap style={{ marginBottom: 12 }}>
      <Segmented
        aria-label={t('pages.clients.insights.period')}
        value={period}
        onChange={(v) => setPeriod(v as InsightsPeriod)}
        options={PERIODS.map((p) => ({
          value: p,
          label: p === 'custom' ? t('pages.clients.insights.custom') : p,
        }))}
      />
      {period === 'custom' && (
        <DatePicker.RangePicker
          aria-label={t('pages.clients.insights.custom')}
          showTime
          onChange={(v) => setCustom(v?.[0] && v?.[1] ? [v[0].unix(), v[1].unix()] : null)}
        />
      )}
    </Space>
  );

  const series: TrafficSeries[] =
    (split === 'node' ? traffic.data?.nodes : traffic.data?.inbounds) ?? [];

  const trafficTab = (
    <>
      {periodBar}
      <Space style={{ marginBottom: 12, marginInlineStart: 12 }}>
        <Segmented
          aria-label={t('pages.clients.insights.split')}
          value={split}
          onChange={(v) => setSplit(v as 'node' | 'inbound')}
          options={[
            { value: 'node', label: t('pages.clients.insights.byNode') },
            { value: 'inbound', label: t('pages.clients.insights.byInbound') },
          ]}
        />
        {traffic.data && (
          <span>
            <Tag color="blue">↑ {SizeFormatter.sizeFormat(traffic.data.total.up)}</Tag>
            <Tag color="green">↓ {SizeFormatter.sizeFormat(traffic.data.total.down)}</Tag>
          </span>
        )}
      </Space>
      {series.length === 0 && !traffic.isLoading ? (
        <Empty description={t('pages.clients.insights.noData')} />
      ) : (
        series.map((s) => {
          const d = traffic.data!;
          const filled = fillSeries(s.points, d.from, d.to, d.step);
          return (
            <div key={`${s.nodeId}-${s.name}`} style={{ marginBottom: 16 }}>
              <div>
                <b>{s.name || `#${s.nodeId}`}</b>{' '}
                <span className="hint">
                  <Tag color="blue">{t('pages.clients.insights.upload')}</Tag>
                  <Tag color="green">{t('pages.clients.insights.download')}</Tag>
                </span>
              </div>
              <Sparkline
                data={filled.up}
                data2={filled.down}
                stroke="#1677ff"
                stroke2="#52c41a"
                name1={t('pages.clients.insights.upload')}
                name2={t('pages.clients.insights.download')}
                labels={filled.times.map((x) => timeLabel(x, d.step))}
                height={120}
                showGrid
                showAxes
                tickCountX={5}
                maxPoints={filled.times.length || 1}
                fillOpacity={0.15}
                showTooltip
                valueMax={null}
                yFormatter={(v) => SizeFormatter.sizeFormat(v)}
                tooltipFormatter={(v) => SizeFormatter.sizeFormat(v)}
              />
            </div>
          );
        })
      )}
    </>
  );

  const activeColumns: TableColumnsType<ActiveConnection> = [
    { title: t('pages.clients.insights.sourceIp'), dataIndex: 'sourceIp' },
    { title: t('pages.clients.insights.destination'), dataIndex: 'dest', ellipsis: true },
    { title: t('pages.clients.insights.port'), dataIndex: 'port', width: 80 },
    { title: t('pages.clients.insights.protocol'), dataIndex: 'network', width: 80 },
    { title: t('pages.clients.insights.node'), dataIndex: 'node' },
    { title: t('pages.clients.insights.inbound'), dataIndex: 'inbound' },
    {
      title: t('pages.clients.insights.started'),
      dataIndex: 'start',
      render: (v: number) => dayjs.unix(v).format('HH:mm:ss'),
      sorter: sizeSorter<ActiveConnection>('start'),
      defaultSortOrder: 'descend',
    },
    {
      title: '↑',
      dataIndex: 'up',
      render: (v: number) => SizeFormatter.sizeFormat(v),
      sorter: sizeSorter<ActiveConnection>('up'),
    },
    {
      title: '↓',
      dataIndex: 'down',
      render: (v: number) => SizeFormatter.sizeFormat(v),
      sorter: sizeSorter<ActiveConnection>('down'),
    },
  ];

  const deviceColumns: TableColumnsType<ClientDevice> = [
    { title: 'IP', dataIndex: 'ip' },
    { title: t('pages.clients.insights.node'), dataIndex: 'node' },
    {
      title: t('pages.clients.insights.lastSeen'),
      dataIndex: 'lastSeen',
      render: (v: number) => dayjs.unix(v).format('YYYY-MM-DD HH:mm:ss'),
      sorter: sizeSorter<ClientDevice>('lastSeen'),
      defaultSortOrder: 'descend',
    },
  ];

  const destColumns: TableColumnsType<DestinationRow> = [
    {
      title: t('pages.clients.insights.destination'),
      dataIndex: 'dest',
      ellipsis: true,
      sorter: true,
      sortOrder: sort === 'dest' ? (order === 'asc' ? 'ascend' : 'descend') : null,
    },
    { title: t('pages.clients.insights.protocol'), dataIndex: 'network', width: 80 },
    { title: t('pages.clients.insights.node'), dataIndex: 'node' },
    {
      title: '↑',
      dataIndex: 'up',
      sorter: true,
      sortOrder: sort === 'up' ? (order === 'asc' ? 'ascend' : 'descend') : null,
      render: (v: number) => SizeFormatter.sizeFormat(v),
    },
    {
      title: '↓',
      dataIndex: 'down',
      sorter: true,
      sortOrder: sort === 'down' ? (order === 'asc' ? 'ascend' : 'descend') : null,
      render: (v: number) => SizeFormatter.sizeFormat(v),
    },
    {
      title: t('pages.clients.insights.total'),
      dataIndex: 'total',
      sorter: true,
      sortOrder: sort === 'total' ? (order === 'asc' ? 'ascend' : 'descend') : null,
      render: (v: number) => SizeFormatter.sizeFormat(v),
    },
    {
      title: t('pages.clients.insights.connections'),
      dataIndex: 'conns',
      sorter: true,
      sortOrder: sort === 'conns' ? (order === 'asc' ? 'ascend' : 'descend') : null,
    },
  ];

  const destinationsTab = (
    <>
      {periodBar}
      <Space wrap style={{ marginBottom: 12 }}>
        <Input.Search
          allowClear
          aria-label={t('pages.clients.insights.search')}
          placeholder={t('pages.clients.insights.search')}
          onSearch={setSearch}
          style={{ width: 240 }}
        />
        <Select
          allowClear
          aria-label={t('pages.clients.insights.allNodes')}
          placeholder={t('pages.clients.insights.allNodes')}
          style={{ minWidth: 160 }}
          options={nodeOptions}
          value={nodeId}
          onChange={(v) => setNodeId(v ?? undefined)}
        />
        <Button
          icon={<DownloadOutlined aria-hidden />}
          disabled={!dests.data?.length}
          onClick={() =>
            FileManager.downloadTextFile(
              destinationsToCsv(dests.data ?? []),
              `${email}-destinations.csv`,
              {
                type: 'text/csv;charset=utf-8',
              },
            )
          }
        >
          CSV
        </Button>
      </Space>
      <Table<DestinationRow>
        size="small"
        rowKey={(r) => `${r.nodeId}|${r.network}|${r.dest}`}
        loading={dests.isLoading}
        columns={destColumns}
        dataSource={dests.data ?? []}
        pagination={{ pageSize: 20, showSizeChanger: false }}
        scroll={{ x: 'max-content' }}
        onChange={(_, __, sorter) => {
          const s = Array.isArray(sorter) ? sorter[0] : sorter;
          if (s?.field && s.order) {
            setSort(s.field as DestinationsFilter['sort']);
            setOrder(s.order === 'ascend' ? 'asc' : 'desc');
          } else {
            setSort('total');
            setOrder('desc');
          }
        }}
      />
    </>
  );

  return (
    <Modal
      open={open}
      title={`${t('pages.clients.insights.title')} — ${email}`}
      footer={null}
      width={1000}
      destroyOnHidden
      onCancel={onClose}
    >
      <Tabs
        activeKey={tab}
        onChange={setTab}
        items={[
          { key: 'traffic', label: t('pages.clients.insights.traffic'), children: trafficTab },
          {
            key: 'active',
            label: `${t('pages.clients.insights.active')}${active.data ? ` (${active.data.length})` : ''}`,
            children: (
              <Table<ActiveConnection>
                size="small"
                rowKey={(r) =>
                  `${r.nodeId}|${r.sourceIp}|${r.dest}|${r.port}|${r.start}|${r.inbound}`
                }
                loading={active.isLoading}
                columns={activeColumns}
                dataSource={active.data ?? []}
                pagination={{ pageSize: 20, showSizeChanger: false }}
                scroll={{ x: 'max-content' }}
                locale={{ emptyText: t('pages.clients.insights.noActive') }}
              />
            ),
          },
          {
            key: 'destinations',
            label: t('pages.clients.insights.destinations'),
            children: destinationsTab,
          },
          {
            key: 'devices',
            label: `${t('pages.clients.insights.devices')}${devices.data ? ` (${devices.data.length})` : ''}`,
            children: (
              <Table<ClientDevice>
                size="small"
                rowKey={(r) => `${r.nodeId}|${r.ip}`}
                loading={devices.isLoading}
                columns={deviceColumns}
                dataSource={devices.data ?? []}
                pagination={false}
                locale={{ emptyText: t('pages.clients.insights.noDevices') }}
              />
            ),
          },
        ]}
      />
    </Modal>
  );
}
