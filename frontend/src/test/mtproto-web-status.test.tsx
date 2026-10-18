import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { FormProvider, useForm } from 'react-hook-form';
import MtprotoWebStatus from '@/pages/inbounds/form/protocols/MtprotoWebStatus';
import { HttpUtil } from '@/utils';

function Harness({ domain = '' }: { domain?: string }) {
  const methods = useForm({ defaultValues: { settings: { webProxy: { domain } } } });
  return (
    <FormProvider {...methods}>
      <input aria-label="domain" {...methods.register('settings.webProxy.domain')} />
      <MtprotoWebStatus />
    </FormProvider>
  );
}
const ready = {
  defaultDomain: 'panel.example.com',
  supported: true,
  port443Available: true,
  port443Free: true,
};

describe('MTProto WEB automation', () => {
  it('suggests the panel domain and preserves a saved custom domain', async () => {
    vi.mocked(HttpUtil.get).mockResolvedValue({ success: true, msg: '', obj: ready });
    const first = render(<Harness />);
    await waitFor(() =>
      expect(screen.getByLabelText('domain')).toHaveProperty('value', 'panel.example.com'),
    );
    expect(screen.getByText(/Default domain:/).textContent).toContain('panel.example.com');
    expect(screen.getByText('TCP 443 is free')).toBeTruthy();
    first.unmount();
    render(<Harness domain="custom.example.com" />);
    await waitFor(() => expect(HttpUtil.get).toHaveBeenCalledTimes(2));
    expect(screen.getByLabelText('domain')).toHaveProperty('value', 'custom.example.com');
  });

  it('does not use an IP address as a public certificate domain', async () => {
    vi.mocked(HttpUtil.get).mockResolvedValue({
      success: true,
      msg: '',
      obj: { ...ready, defaultDomain: '203.0.113.10' },
    });
    render(<Harness />);
    await waitFor(() => expect(HttpUtil.get).toHaveBeenCalled());
    expect(screen.getByLabelText('domain')).toHaveProperty('value', '');
  });

  it('preserves an edit while loading and reports port conflicts', async () => {
    let resolve!: (value: Awaited<ReturnType<typeof HttpUtil.get>>) => void;
    vi.mocked(HttpUtil.get).mockImplementationOnce(
      () =>
        new Promise((done) => {
          resolve = done;
        }) as ReturnType<typeof HttpUtil.get>,
    );
    render(<Harness />);
    fireEvent.change(screen.getByLabelText('domain'), { target: { value: 'edited.example.com' } });
    resolve({
      success: true,
      msg: '',
      obj: { ...ready, port443Available: false, port443Owner: 'x-ui' },
    });
    await screen.findByText(/TCP 443.*x-ui/);
    expect(screen.getByLabelText('domain')).toHaveProperty('value', 'edited.example.com');
  });
  it('rechecks port availability and can explicitly restore the default domain', async () => {
    vi.mocked(HttpUtil.get)
      .mockResolvedValueOnce({ success: true, msg: '', obj: ready })
      .mockResolvedValueOnce({
        success: true,
        msg: '',
        obj: { ...ready, port443Free: false, port443Available: false, port443Owner: 'nginx' },
      });
    render(<Harness domain="custom.example.com" />);
    fireEvent.click(await screen.findByRole('button', { name: 'Use default domain' }));
    expect(screen.getByLabelText('domain')).toHaveProperty('value', 'panel.example.com');
    fireEvent.click(screen.getByRole('button', { name: 'Check WEB Proxy readiness' }));
    await screen.findByText(/TCP 443.*nginx/);
    expect(screen.queryByText('TCP 443 is free')).toBeNull();
  });
});
