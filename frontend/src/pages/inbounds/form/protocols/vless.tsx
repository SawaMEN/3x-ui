import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Form, Input, Select, Space, Typography } from 'antd';

import { FormField } from '@/components/form/rhf';
import { VLESS_AUTH_LABEL_KEYS, type VlessAuthKind } from '@/lib/xray/vless-encryption';

interface VlessFieldsProps {
  saving: boolean;
  selectedVlessAuth: string;
  vlessAuthKind: VlessAuthKind | null;
  network: string;
  security: string;
  shadowTlsEnabled?: boolean;
  getNewVlessEnc: (kind: VlessAuthKind) => void;
  clearVlessEnc: () => void;
}

export default function VlessFields({
  saving,
  selectedVlessAuth,
  vlessAuthKind,
  shadowTlsEnabled = false,
  getNewVlessEnc,
  clearVlessEnc,
}: VlessFieldsProps) {
  const { t } = useTranslation();
  const [authKind, setAuthKind] = useState<VlessAuthKind>(vlessAuthKind ?? 'x25519');

  const [syncedAuthKind, setSyncedAuthKind] = useState(vlessAuthKind);
  if (vlessAuthKind !== syncedAuthKind) {
    setSyncedAuthKind(vlessAuthKind);
    setAuthKind(vlessAuthKind ?? 'x25519');
  }

  const authOptions = (Object.entries(VLESS_AUTH_LABEL_KEYS) as [VlessAuthKind, string][]).map(
    ([value, labelKey]) => ({ value, label: t(labelKey) }),
  );

  return (
    <>
      {!shadowTlsEnabled && (
        <>
          <FormField name={['settings', 'decryption']} label={t('pages.inbounds.decryption')}>
            <Input />
          </FormField>
          <FormField name={['settings', 'encryption']} label={t('pages.inbounds.encryption')}>
            <Input />
          </FormField>
        </>
      )}
      {!shadowTlsEnabled && (
        <Form.Item label={t('pages.inbounds.vlessAuthGenerate')}>
          <Space size={8} wrap>
            <Select
              value={authKind}
              onChange={(v) => setAuthKind(v)}
              options={authOptions}
              style={{ width: 240 }}
            />
            <Button type="primary" loading={saving} onClick={() => getNewVlessEnc(authKind)}>
              {t('pages.inbounds.vlessAuthGenerateButton')}
            </Button>
            <Button danger onClick={clearVlessEnc}>
              {t('clear')}
            </Button>
          </Space>
          <Typography.Text type="secondary" className="vless-auth-state">
            {t('pages.inbounds.vlessAuthSelected', { auth: selectedVlessAuth })}
          </Typography.Text>
        </Form.Item>
      )}
    </>
  );
}
