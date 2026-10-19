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
    fireEvent.click(await screen.findByText('Choose from ready-made catalogs'));
    expect(screen.queryByText('Choose HTML file')).toBeNull();
    expect(screen.queryByLabelText('Page HTML')).toBeNull();
    fireEvent.click(screen.getByText('Save'));
    expect(save).toHaveBeenLastCalledWith({
      site: { mode: 'catalog', catalog: 'builtin', template: 'notes' },
    });
    fireEvent.mouseDown(screen.getByLabelText('Catalog'));
    fireEvent.click(await screen.findByText('MTProxyL collection'));
    fireEvent.click(screen.getByText('Save'));
    expect(save).toHaveBeenLastCalledWith({
      site: { mode: 'catalog', catalog: 'mtproxyl', template: 'stub' },
    });
    fireEvent.mouseDown(screen.getByLabelText('Ready-made page'));
    fireEvent.click(await screen.findByText('Cat Runner — cat game'));
    fireEvent.click(screen.getByText('Save'));
    expect(save).toHaveBeenLastCalledWith({
      site: { mode: 'catalog', catalog: 'mtproxyl', template: 'catrunner' },
    });
    fireEvent.mouseDown(screen.getByLabelText('Catalog'));
    fireEvent.click(await screen.findByText('3x-ui pages'));
    fireEvent.click(screen.getByText('Save'));
    expect(save).toHaveBeenLastCalledWith({
      site: { mode: 'catalog', catalog: 'builtin', template: 'notes' },
    });
  });
});
