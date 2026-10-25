import { fireEvent, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import SubHeader from '@/pages/sub/SubHeader';
import { renderWithProviders } from './test-utils';

describe('subscription theme selection', () => {
  it('can leave Cyberpunk and explicitly choose WhiteMAX', async () => {
    const onWhiteMaxChange = vi.fn();
    renderWithProviders(
      <SubHeader
        title="Test"
        sId="id"
        email=""
        lang="en-US"
        onLangChange={vi.fn()}
        whiteMax={false}
        onWhiteMaxChange={onWhiteMaxChange}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: 'Theme' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Light' }));
    await waitFor(() => expect(document.documentElement.dataset.theme).toBe('light'));
    expect(onWhiteMaxChange).toHaveBeenCalledWith(false);
    fireEvent.click(screen.getByRole('button', { name: 'Theme' }));
    fireEvent.click(await screen.findByRole('menuitem', { name: 'WhiteMax' }));
    expect(onWhiteMaxChange).toHaveBeenCalledWith(true);
  });
});
