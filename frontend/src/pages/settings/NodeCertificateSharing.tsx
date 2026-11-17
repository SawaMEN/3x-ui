import { Alert, Drawer, Space } from 'antd';
import { useTranslation } from 'react-i18next';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import CoverCertificateFleet from '@/pages/manager/CoverCertificateFleet';

// Mounted only while the dialog is open: inventory and delivery polling stop
// when it closes, and remain separate from the domain configuration draft.
export default function NodeCertificateSharing({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const { nodes, loading, fetched, fetchError } = useNodesQuery();
  const label = (key: string) => t('pages.settings.domains.workspace.' + key);
  return (
    <Drawer open title={label('takeNodeCertificate')} onClose={onClose} size={860} destroyOnHidden>
      <Space orientation="vertical" style={{ width: '100%' }}>
        {fetchError && <Alert type="error" showIcon title={fetchError} />}
        {loading && !fetched && <Alert type="info" showIcon title={label('loadingNodes')} />}
        {fetched && !fetchError && <CoverCertificateFleet nodes={nodes} defaultRecipientId={0} />}
      </Space>
    </Drawer>
  );
}
