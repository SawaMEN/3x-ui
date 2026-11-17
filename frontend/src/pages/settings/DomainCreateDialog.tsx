import { Modal, Typography } from 'antd';
import {
  GlobalOutlined,
  LinkOutlined,
  CloudOutlined,
  SafetyCertificateOutlined,
  ApartmentOutlined,
  LayoutOutlined,
  ArrowRightOutlined,
} from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import type { DomainPreset } from './DomainOverview';
export default function DomainCreateDialog({
  open,
  onClose,
  onChoose,
}: {
  open: boolean;
  onClose: () => void;
  onChoose: (preset: DomainPreset) => void;
}) {
  const { t } = useTranslation();
  const l = (k: string) => t('pages.settings.domains.' + k);
  const options = [
    ['subscription', <LinkOutlined key="subscription" />],
    ['direct', <GlobalOutlined key="direct" />],
    ['cdn', <CloudOutlined key="cdn" />],
    ['sni', <LayoutOutlined key="sni" />],
    ['shared', <ApartmentOutlined key="shared" />],
    ['certificate', <SafetyCertificateOutlined key="certificate" />],
  ] as const;
  return (
    <Modal
      open={open}
      title={l('redesign.createTitle')}
      onCancel={onClose}
      footer={null}
      width={720}
    >
      <Typography.Paragraph type="secondary">{l('redesign.createHint')}</Typography.Paragraph>
      <div className="domain-create-grid">
        {options.map(([key, icon]) => (
          <button
            type="button"
            key={key}
            aria-label={l('workspace.preset.' + key)}
            onClick={() => onChoose(key)}
            className="domain-create-option"
          >
            <span className="domain-create-icon">{icon}</span>
            <strong>{l('workspace.preset.' + key)}</strong>
            <span className="domain-create-description">{l('workspace.hint.' + key)}</span>
            <ArrowRightOutlined className="domain-create-arrow" />
          </button>
        ))}
      </div>
    </Modal>
  );
}
