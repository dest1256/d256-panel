import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Form, Input, Modal } from 'antd';

import type { NodeRecord } from '@/api/queries/useNodesQuery';
import type { NodeFormPayload } from '@/api/queries/useNodeMutations';

interface NodeFormModalProps {
  open: boolean;
  mode: 'add' | 'edit';
  node: NodeRecord | null;
  save: (payload: NodeFormPayload) => Promise<{ success?: boolean } | undefined>;
  onOpenChange: (open: boolean) => void;
}

// Name, remark and the public address a node advertises in subscriptions. The link itself is
// configured by enrollment, so there are no connection settings here.
export default function NodeFormModal({
  open,
  mode,
  node,
  save,
  onOpenChange,
}: NodeFormModalProps) {
  const { t } = useTranslation();
  const [form] = Form.useForm<NodeFormPayload>();
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    form.setFieldsValue({
      name: node?.name ?? '',
      remark: node?.remark ?? '',
      address: node?.address ?? '',
    });
  }, [open, node, form]);

  const onOk = async () => {
    const values = await form.validateFields();
    setSaving(true);
    try {
      const msg = await save({ ...values, remark: values.remark ?? '' });
      if (msg?.success) onOpenChange(false);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      open={open}
      title={mode === 'add' ? t('pages.nodes.addNode') : t('pages.nodes.editNode')}
      okText={mode === 'add' ? t('pages.nodes.createAndEnroll') : t('save')}
      cancelText={t('cancel')}
      confirmLoading={saving}
      onOk={onOk}
      onCancel={() => onOpenChange(false)}
      destroyOnHidden
    >
      <Form form={form} layout="vertical">
        <Form.Item
          name="name"
          label={t('pages.nodes.name')}
          rules={[
            {
              required: true,
              whitespace: true,
              max: 64,
              message: t('pages.nodes.toasts.fillRequired'),
            },
          ]}
        >
          <Input placeholder={t('pages.nodes.namePlaceholder')} />
        </Form.Item>
        <Form.Item name="remark" label={t('pages.nodes.remark')} rules={[{ max: 256 }]}>
          <Input />
        </Form.Item>
        <Form.Item
          name="address"
          label={t('pages.nodes.address')}
          extra={t('pages.nodes.addressHint')}
          rules={[
            { required: true, whitespace: true, message: t('pages.nodes.toasts.fillRequired') },
            {
              pattern: /^[A-Za-z0-9]([A-Za-z0-9.:-]{0,251}[A-Za-z0-9])?$/,
              message: t('pages.nodes.addressInvalid'),
            },
          ]}
        >
          <Input placeholder={t('pages.nodes.addressPlaceholder')} />
        </Form.Item>
      </Form>
    </Modal>
  );
}
