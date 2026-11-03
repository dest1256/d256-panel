import { lazy, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  ConfigProvider,
  Layout,
  Result,
  Space,
  Spin,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  AreaChartOutlined,
  CloudDownloadOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  HddOutlined,
  SwapOutlined,
} from '@ant-design/icons';

import { CPUFormatter, SizeFormatter } from '@/utils';
import { useTheme } from '@/hooks/useTheme';
import { useStatusQuery } from '@/api/queries/useStatusQuery';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import AppSidebar from '@/layouts/AppSidebar';
import { LazyMount } from '@/components/utility';
import { setMessageInstance } from '@/utils/messageBus';
import VitalTile from './VitalTile';
import NodesOverview from './NodesOverview';
import { mean, peak, useOverviewHistory } from './useOverviewHistory';
const BackupModal = lazy(() => import('./BackupModal'));
const SystemHistoryModal = lazy(() => import('./SystemHistoryModal'));
import './IndexPage.css';

export default function IndexPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { status, fetched, fetchError, refresh } = useStatusQuery();
  const { isMobile } = useMediaQuery();
  const [messageApi, messageContextHolder] = message.useMessage();
  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const basePath = window.X_UI_BASE_PATH || '';
  const [backupOpen, setBackupOpen] = useState(false);
  const [sysHistoryOpen, setSysHistoryOpen] = useState(false);
  const [loading, setLoading] = useState(false);

  const history = useOverviewHistory(status, fetched && !fetchError);
  const pageClass = `index-page ${isDark ? 'is-dark' : ''} ${isUltra ? 'is-ultra' : ''}`.trim();
  const freeDisk = Math.max(0, status.disk.total - status.disk.current);
  const version = useMemo(() => window.X_UI_CUR_VER || '', []);

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />

        <Layout className="content-shell">
          <Layout.Content className="content-area">
            <Spin
              spinning={loading || !fetched}
              delay={200}
              description={t('loading')}
              size="large"
            >
              {!fetched ? (
                <div className="loading-spacer" />
              ) : fetchError ? (
                <Result
                  status="error"
                  title={t('somethingWentWrong')}
                  subTitle={fetchError}
                  extra={
                    <Button type="primary" onClick={refresh}>
                      {t('refresh')}
                    </Button>
                  }
                />
              ) : (
                <div className="ov-page">
                  <Space
                    wrap
                    style={{ marginBottom: 12, justifyContent: 'space-between', width: '100%' }}
                  >
                    <Space>
                      <Typography.Title level={4} style={{ margin: 0 }}>
                        {t('pages.index.title')}
                      </Typography.Title>
                      {version ? <Tag>{version}</Tag> : null}
                    </Space>
                    <Space wrap>
                      <Button
                        icon={<AreaChartOutlined aria-hidden />}
                        onClick={() => setSysHistoryOpen(true)}
                      >
                        {!isMobile && t('pages.index.systemHistory')}
                      </Button>
                      <Button
                        icon={<CloudDownloadOutlined aria-hidden />}
                        onClick={() => setBackupOpen(true)}
                      >
                        {!isMobile && t('pages.index.backup')}
                      </Button>
                    </Space>
                  </Space>

                  <NodesOverview />

                  <hr className="ov-rule" />
                  <Typography.Title level={5}>{t('pages.index.panelHost')}</Typography.Title>
                  <div className="ov-vitals">
                    <VitalTile
                      icon={<DashboardOutlined aria-hidden />}
                      label={t('pages.index.cpu')}
                      available={status.metricAvailable('cpu')}
                      percent={status.cpu.percent}
                      statusColor={status.cpu.color}
                      detail={`${CPUFormatter.cpuCoreFormat(status.cpuCores)} · ${CPUFormatter.cpuSpeedFormat(status.cpuSpeedMhz)}`}
                      footLeft={`${t('pages.index.avg')} ${mean(history.series.cpu).toFixed(0)}%`}
                      footRight={`${t('pages.index.peak')} ${peak(history.series.cpu).toFixed(0)}%`}
                      data={history.series.cpu}
                      isMobile={isMobile}
                    />
                    <VitalTile
                      icon={<DatabaseOutlined aria-hidden />}
                      label={t('pages.index.memory')}
                      percent={status.mem.percent}
                      statusColor={status.mem.color}
                      detail={`${SizeFormatter.sizeFormat(status.mem.current)} / ${SizeFormatter.sizeFormat(status.mem.total)}`}
                      footLeft={`${t('pages.index.avg')} ${mean(history.series.mem).toFixed(0)}%`}
                      footRight={`${t('pages.index.peak')} ${peak(history.series.mem).toFixed(0)}%`}
                      data={history.series.mem}
                      isMobile={isMobile}
                    />
                    <VitalTile
                      icon={<SwapOutlined aria-hidden />}
                      label={t('pages.index.swap')}
                      available={status.metricAvailable('swap')}
                      percent={status.swap.percent}
                      statusColor={status.swap.color}
                      detail={`${SizeFormatter.sizeFormat(status.swap.current)} / ${SizeFormatter.sizeFormat(status.swap.total)}`}
                      footLeft={`${t('pages.index.avg')} ${mean(history.series.swap).toFixed(1)}%`}
                      footRight={`${t('pages.index.peak')} ${peak(history.series.swap).toFixed(0)}%`}
                      data={history.series.swap}
                      isMobile={isMobile}
                    />
                    <VitalTile
                      icon={<HddOutlined aria-hidden />}
                      label={t('pages.index.storage')}
                      percent={status.disk.percent}
                      statusColor={status.disk.color}
                      detail={`${SizeFormatter.sizeFormat(status.disk.current)} / ${SizeFormatter.sizeFormat(status.disk.total)}`}
                      footLeft={`${t('pages.index.free')} ${SizeFormatter.sizeFormat(freeDisk)}`}
                      footRight={`${t('pages.index.avg')} ${mean(history.series.diskUsage).toFixed(1)}%`}
                      data={history.series.diskUsage}
                      isMobile={isMobile}
                    />
                  </div>
                </div>
              )}
            </Spin>
          </Layout.Content>
        </Layout>

        <LazyMount when={backupOpen}>
          <BackupModal
            open={backupOpen}
            basePath={basePath}
            onClose={() => setBackupOpen(false)}
            onBusy={({ busy }) => setLoading(busy)}
          />
        </LazyMount>
        <LazyMount when={sysHistoryOpen}>
          <SystemHistoryModal
            open={sysHistoryOpen}
            status={status}
            onClose={() => setSysHistoryOpen(false)}
          />
        </LazyMount>
      </Layout>
    </ConfigProvider>
  );
}
