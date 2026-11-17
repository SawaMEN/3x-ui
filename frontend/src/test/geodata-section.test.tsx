import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import GeodataSection from '@/pages/index/GeodataSection';

const { refetch, useFiles } = vi.hoisted(() => ({ refetch: vi.fn(), useFiles: vi.fn() }));
vi.mock('@/api/queries/useGeodata', () => ({ useGeodataFiles: useFiles }));
vi.mock('@/components/geodata/GeoBrowserModal', () => ({
  default: () => <div>Database browser</div>,
}));

describe('GeodataSection', () => {
  it('browses installed databases and refreshes actual state', async () => {
    useFiles.mockReturnValue({
      data: [
        { name: 'geosite.dat', categories: 2 },
        { name: 'broken.dat', error: 'invalid database' },
      ],
      isLoading: false,
      isError: false,
      refetch,
    });
    render(<GeodataSection active onBusy={vi.fn()} onClose={vi.fn()} />);
    expect(useFiles).toHaveBeenCalledWith(true);
    expect(screen.getByText('geosite.dat')).toBeTruthy();
    expect(screen.getByText('invalid database')).toBeTruthy();
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Refresh' }));
    expect(refetch).toHaveBeenCalledOnce();
    await user.click(screen.getByRole('button', { name: 'Geosite' }));
    expect(screen.getByText('Database browser')).toBeTruthy();
    expect(screen.queryByRole('textbox')).toBeNull();
  });
});
