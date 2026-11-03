import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Modal, Select, Upload, message } from 'antd';
import { UploadOutlined } from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import type { NodeRecord } from '@/api/queries/useNodesQuery';

interface ImportResult {
  inbounds: number;
  clients: number;
  skipped: { remark: string; port: number; reason: string }[];
}

interface ImportXuiModalProps {
  open: boolean;
  nodes: NodeRecord[];
  onClose: () => void;
  onDone: () => void;
}

export default function ImportXuiModal({ open, nodes, onClose, onDone }: ImportXuiModalProps) {
  const { t } = useTranslation();
  const [nodeId, setNodeId] = useState<number | undefined>(undefined);
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<ImportResult | null>(null);
  const [messageApi, holder] = message.useMessage();

  async function run() {
    if (!nodeId || !file) return;
    setBusy(true);
    try {
      const form = new FormData();
      form.append('db', file);
      const msg = await HttpUtil.post<ImportResult>(`/panel/api/nodes/importXui/${nodeId}`, form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      if (msg?.success && msg.obj) {
        setResult(msg.obj);
        messageApi.success(
          t('pages.nodes.importXuiDone', { inbounds: msg.obj.inbounds, clients: msg.obj.clients }),
        );
        onDone();
      }
    } finally {
      setBusy(false);
    }
  }

  function close() {
    setResult(null);
    setFile(null);
    onClose();
  }

  return (
    <Modal
      open={open}
      title={t('pages.nodes.importXui')}
      onCancel={close}
      destroyOnHidden
      footer={[
        <Button key="close" onClick={close}>
          {t('close')}
        </Button>,
        <Button key="run" type="primary" loading={busy} disabled={!nodeId || !file} onClick={run}>
          {t('pages.nodes.importXui')}
        </Button>,
      ]}
    >
      {holder}
      <p>{t('pages.nodes.importXuiHint')}</p>
      <Select
        aria-label={t('pages.nodes.importXuiNode')}
        placeholder={t('pages.nodes.importXuiNode')}
        style={{ width: '100%', marginBottom: 12 }}
        value={nodeId}
        onChange={setNodeId}
        options={nodes.map((n) => ({ value: n.id, label: n.name }))}
      />
      <Upload
        accept=".db"
        maxCount={1}
        beforeUpload={(f) => {
          setFile(f);
          return false;
        }}
        onRemove={() => setFile(null)}
      >
        <Button icon={<UploadOutlined aria-hidden />}>{t('pages.nodes.importXuiFile')}</Button>
      </Upload>
      {result && result.skipped.length > 0 && (
        <Alert
          style={{ marginTop: 12 }}
          type="warning"
          showIcon
          title={`${t('pages.nodes.importXuiSkipped')}: ${result.skipped.length}`}
          description={
            <ul style={{ margin: 0, paddingInlineStart: 18 }}>
              {result.skipped.map((s, i) => (
                <li key={i}>{`${s.remark || s.port} — ${s.reason}`}</li>
              ))}
            </ul>
          }
        />
      )}
    </Modal>
  );
}
