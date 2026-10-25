import { fireEvent, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import SubHeader from '@/pages/sub/SubHeader';
import { renderWithProviders } from './test-utils';

describe('public subscription themes', () => {
  it('switches from the default Cyberpunk theme through every available palette', async () => {
    renderWithProviders(
      <SubHeader
        whiteMax={false}
        onWhiteMaxChange={vi.fn()}
        title="VPN"
        sId="test"
        email=""
        lang="en-US"
        onLangChange={vi.fn()}
      />,
    );
    for (const [label, mode] of [
      ['Blue Gray', 'blue-gray'],
      ['Ultra Dark', 'ultra-dark'],
      ['Colorful', 'colorful'],
      ['Light', 'light'],
      ['Dark', 'dark'],
      ['Cyberpunk', 'cyberpunk'],
    ]) {
      fireEvent.click(screen.getByRole('button', { name: 'Theme' }));
      fireEvent.click(await screen.findByRole('menuitem', { name: label }));
      await waitFor(() => expect(document.documentElement.getAttribute('data-theme')).toBe(mode));
      expect(localStorage.getItem('xui-theme')).toBe(mode);
    }
  });
});
