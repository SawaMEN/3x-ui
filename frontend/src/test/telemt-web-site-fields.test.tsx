import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { FormProvider, useForm } from 'react-hook-form';

import TelemtWebSiteFields from '@/pages/telemt/TelemtWebSiteFields';
import type { TelemtWebSite } from '@/schemas/telemt-web';

function Harness({ save }: { save: (value: unknown) => void }) {
  const form = useForm<{ site?: TelemtWebSite }>();
  return (
    <FormProvider {...form}>
      <TelemtWebSiteFields />
      <button onClick={() => save(form.getValues())}>Save</button>
    </FormProvider>
  );
}

describe('optional Telemt cover site', () => {
  it('leaves the default untouched and persists only an explicitly chosen site', async () => {
    const save = vi.fn();
    render(<Harness save={save} />);
    fireEvent.click(screen.getByText('Save'));
    expect(save).toHaveBeenLastCalledWith({});
    fireEvent.click(screen.getByText('Cover website: optional customization'));
    fireEvent.mouseDown(screen.getByLabelText('Website source'));
    fireEvent.click(await screen.findByText('Generate a themed page'));
    fireEvent.change(screen.getByLabelText('Title (optional)'), {
      target: { value: 'My notebook' },
    });
    fireEvent.click(screen.getByText('Save'));
    expect(save).toHaveBeenLastCalledWith({
      site: { mode: 'generate', theme: 'notes', title: 'My notebook', description: undefined },
    });
    fireEvent.mouseDown(screen.getByLabelText('Website source'));
    fireEvent.click(await screen.findByText('Upload ready-made HTML'));
    const html = '<html><body>Downloaded template</body></html>';
    fireEvent.change(screen.getByLabelText('Page HTML'), { target: { value: html } });
    fireEvent.click(screen.getByText('Save'));
    expect(save).toHaveBeenLastCalledWith({ site: { mode: 'html', html } });
  });
});
