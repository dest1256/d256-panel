import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Input, Modal, Typography } from 'antd';
import { CopyOutlined } from '@ant-design/icons';

import { ClipboardManager } from '@/utils';

interface EnrollModalProps {
  open: boolean;
  nodeName: string;
  token: string;
  onClose: () => void;
}

// The panel URL a node enrolls against: this page's origin plus the panel base path.
function panelUrl(): string {
  const base = (window.X_UI_BASE_PATH || '/').replace(/\/+$/, '');
  return `${window.location.origin}${base}`;
}

export default function EnrollModal({ open, nodeName, token, onClose }: EnrollModalProps) {
  const { t } = useTranslation();
  // The token goes through stdin so it stays out of shell history and the process list.
  const command = useMemo(() => `echo '${token}' | d256-node enroll ${panelUrl()} -`, [token]);
  return (
    <Modal
      open={open}
      title={t('pages.nodes.enrollTitle', { name: nodeName })}
      onCancel={onClose}
      footer={
        <Button type="primary" onClick={onClose}>
          {t('close')}
        </Button>
      }
      destroyOnHidden
    >
      <Alert
        type="warning"
        showIcon
        title={t('pages.nodes.enrollTokenOnce')}
        style={{ marginBottom: 12 }}
      />
      <Typography.Paragraph>{t('pages.nodes.enrollIntro')}</Typography.Paragraph>
      <Input.TextArea
        readOnly
        value={command}
        autoSize={{ minRows: 2, maxRows: 4 }}
        style={{ fontFamily: 'monospace' }}
        aria-label={t('pages.nodes.enrollCommand')}
      />
      <Button
        icon={<CopyOutlined aria-hidden />}
        style={{ marginTop: 8 }}
        onClick={() => ClipboardManager.copyText(command)}
      >
        {t('copy')}
      </Button>
    </Modal>
  );
}
