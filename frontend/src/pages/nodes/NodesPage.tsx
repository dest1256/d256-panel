import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  Col,
  ConfigProvider,
  Layout,
  Modal,
  Result,
  Row,
  Spin,
  Statistic,
  message,
} from 'antd';
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  CloudServerOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';

import { useTheme } from '@/hooks/useTheme';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import type { NodeRecord } from '@/api/queries/useNodesQuery';
import { useNodesLiveQuery } from '@/api/queries/useNodesLiveQuery';
import { useNodeMutations } from '@/api/queries/useNodeMutations';
import type { NodeFormPayload } from '@/api/queries/useNodeMutations';
import AppSidebar from '@/layouts/AppSidebar';
import NodeList from './NodeList';
import NodeFormModal from './NodeFormModal';
import EnrollModal from './EnrollModal';
import ImportXuiModal from './ImportXuiModal';
import { setMessageInstance } from '@/utils/messageBus';

export default function NodesPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { isMobile } = useMediaQuery();
  const [modal, modalContextHolder] = Modal.useModal();
  const [messageApi, messageContextHolder] = message.useMessage();
  useEffect(() => {
    setMessageInstance(messageApi);
  }, [messageApi]);

  const { nodes, loading, fetched, fetchError, refetch, totals } = useNodesQuery();
  const { byId: liveById } = useNodesLiveQuery();
  const { create, update, remove, setEnable, newEnrollToken } = useNodeMutations();

  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState<'add' | 'edit'>('add');
  const [formNode, setFormNode] = useState<NodeRecord | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [enroll, setEnroll] = useState<{ name: string; token: string } | null>(null);

  const onAdd = useCallback(() => {
    setFormMode('add');
    setFormNode(null);
    setFormOpen(true);
  }, []);

  const onEdit = useCallback((node: NodeRecord) => {
    setFormMode('edit');
    setFormNode({ ...node });
    setFormOpen(true);
  }, []);

  const onSave = useCallback(
    async (payload: NodeFormPayload) => {
      if (formMode === 'edit' && formNode?.id) return update(formNode.id, payload);
      const msg = await create(payload);
      if (msg?.success && msg.obj) setEnroll({ name: payload.name, token: msg.obj.token });
      return msg;
    },
    [formMode, formNode, update, create],
  );

  const onDelete = useCallback(
    (node: NodeRecord) => {
      modal.confirm({
        title: t('pages.nodes.deleteConfirmTitle', { name: node.name }),
        content: t('pages.nodes.deleteConfirmContent'),
        okText: t('delete'),
        okType: 'danger',
        cancelText: t('cancel'),
        onOk: async () => {
          const msg = await remove(node.id);
          if (msg?.success) messageApi.success(t('pages.nodes.toasts.deleted'));
        },
      });
    },
    [modal, t, remove, messageApi],
  );

  const onNewToken = useCallback(
    (node: NodeRecord) => {
      modal.confirm({
        title: t('pages.nodes.newTokenConfirmTitle', { name: node.name }),
        content: t('pages.nodes.newTokenConfirmContent'),
        okText: t('pages.nodes.newToken'),
        okType: 'danger',
        cancelText: t('cancel'),
        onOk: async () => {
          const msg = await newEnrollToken(node.id);
          if (msg?.success && msg.obj) setEnroll({ name: node.name ?? '', token: msg.obj.token });
        },
      });
    },
    [modal, t, newEnrollToken],
  );

  const onToggleEnable = useCallback(
    async (node: NodeRecord, next: boolean) => {
      await setEnable(node.id, next);
    },
    [setEnable],
  );

  const pageClass = useMemo(() => {
    const classes = ['nodes-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      {modalContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />

        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Spin spinning={!fetched} delay={200} description={t('loading')} size="large">
              {!fetched ? (
                <div className="loading-spacer" />
              ) : fetchError ? (
                <Result
                  status="error"
                  title={t('somethingWentWrong')}
                  subTitle={fetchError}
                  extra={
                    <Button type="primary" loading={loading} onClick={() => refetch()}>
                      {t('refresh')}
                    </Button>
                  }
                />
              ) : (
                <Row gutter={[isMobile ? 8 : 16, isMobile ? 8 : 12]}>
                  <Col span={24}>
                    <Card size="small" hoverable className="summary-card">
                      <Row gutter={[16, isMobile ? 16 : 12]}>
                        <Col xs={12} md={6}>
                          <Statistic
                            title={t('pages.nodes.totalNodes')}
                            value={String(totals.total)}
                            prefix={<CloudServerOutlined aria-hidden />}
                          />
                        </Col>
                        <Col xs={12} md={6}>
                          <Statistic
                            title={t('pages.nodes.onlineNodes')}
                            value={String(totals.online)}
                            prefix={
                              <CheckCircleOutlined
                                aria-hidden
                                style={{ color: 'var(--ant-color-success)' }}
                              />
                            }
                          />
                        </Col>
                        <Col xs={12} md={6}>
                          <Statistic
                            title={t('pages.nodes.offlineNodes')}
                            value={String(totals.offline)}
                            prefix={
                              <CloseCircleOutlined
                                aria-hidden
                                style={{ color: 'var(--ant-color-error)' }}
                              />
                            }
                          />
                        </Col>
                        <Col xs={12} md={6}>
                          <Statistic
                            title={t('pages.nodes.avgLatency')}
                            value={totals.avgLatency > 0 ? `${totals.avgLatency} ms` : '-'}
                            prefix={<ThunderboltOutlined aria-hidden />}
                          />
                        </Col>
                      </Row>
                    </Card>
                  </Col>

                  <Col span={24} style={{ textAlign: 'end' }}>
                    <Button onClick={() => setImportOpen(true)}>
                      {t('pages.nodes.importXui')}
                    </Button>
                  </Col>

                  <Col span={24}>
                    <NodeList
                      nodes={nodes}
                      liveById={liveById}
                      loading={loading}
                      onAdd={onAdd}
                      onEdit={onEdit}
                      onDelete={onDelete}
                      onNewToken={onNewToken}
                      onToggleEnable={onToggleEnable}
                    />
                  </Col>
                </Row>
              )}
            </Spin>
          </Layout.Content>
        </Layout>

        <NodeFormModal
          open={formOpen}
          mode={formMode}
          node={formNode}
          save={onSave}
          onOpenChange={setFormOpen}
        />
        <ImportXuiModal
          open={importOpen}
          nodes={nodes}
          onClose={() => setImportOpen(false)}
          onDone={() => refetch()}
        />
        <EnrollModal
          open={enroll !== null}
          nodeName={enroll?.name ?? ''}
          token={enroll?.token ?? ''}
          onClose={() => setEnroll(null)}
        />
      </Layout>
    </ConfigProvider>
  );
}
