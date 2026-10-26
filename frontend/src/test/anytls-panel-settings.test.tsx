import { fireEvent, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Form } from 'antd';
import { FormProvider, useForm, useWatch } from 'react-hook-form';
import { HttpUtil, Msg } from '@/utils';
import AnyTlsFields from '@/pages/inbounds/form/protocols/anytls';
import { renderWithProviders } from './test-utils';

function Harness({ nodeId }: { nodeId: number | null }) {
  const methods = useForm({
    defaultValues: {
      nodeId,
      settings: {
        tls: {
          serverName: 'old.example',
          certificatePath: '/old/cert',
          keyPath: '/old/key',
          enabled: true,
        },
      },
    },
  });
  const tls = useWatch({ control: methods.control, name: 'settings.tls' });
  return (
    <FormProvider {...methods}>
      <Form>
        <AnyTlsFields />
        <output data-testid="tls-settings">{JSON.stringify(tls)}</output>
      </Form>
    </FormProvider>
  );
}

function clickInsert() {
  fireEvent.click(
    screen.getByRole('button', {
      name: /Insert panel|Вставить настройки панели|anytlsInsertPanelSettings/i,
    }),
  );
}

describe('AnyTLS certificate defaults', () => {
  afterEach(async () => {
    await new Promise((resolve) => setTimeout(resolve, 100));
  });

  it.each([null, 7])(
    'uses the selected server certificate identity for node %s',
    async (nodeId) => {
      const get = vi
        .spyOn(HttpUtil, 'get')
        .mockResolvedValue(
          new Msg(true, '', {
            webCertFile: '/selected/cert',
            webKeyFile: '/selected/key',
            webDomain: 'selected.example',
          }),
        );
      renderWithProviders(<Harness nodeId={nodeId} />);
      clickInsert();
      await waitFor(() =>
        expect(screen.getByTestId('tls-settings').textContent).toContain('selected.example'),
      );
      expect(get).toHaveBeenCalledWith(
        nodeId === null ? '/panel/api/server/getWebCertFiles' : '/panel/api/nodes/webCert/7',
        undefined,
        { silent: true },
      );
      expect(JSON.parse(screen.getByTestId('tls-settings').textContent || '{}')).toMatchObject({
        certificatePath: '/selected/cert',
        keyPath: '/selected/key',
        serverName: 'selected.example',
      });
    },
  );

  it('keeps all existing fields when the selected server has an incomplete certificate pair', async () => {
    vi.spyOn(HttpUtil, 'get').mockResolvedValue(
      new Msg(true, '', { webCertFile: '/partial/cert', webDomain: 'wrong.example' }),
    );
    renderWithProviders(<Harness nodeId={7} />);
    clickInsert();
    await waitFor(() => expect(HttpUtil.get).toHaveBeenCalled());
    expect(JSON.parse(screen.getByTestId('tls-settings').textContent || '{}')).toMatchObject({
      certificatePath: '/old/cert',
      keyPath: '/old/key',
      serverName: 'old.example',
    });
  });
});
