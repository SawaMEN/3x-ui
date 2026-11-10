import { afterEach, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

afterEach(() => {
  vi.restoreAllMocks();
});
it('opens the panel when browser storage access is denied', async () => {
  vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
    throw new DOMException('denied', 'SecurityError');
  });
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
    throw new DOMException('full', 'QuotaExceededError');
  });
  const { ThemeProvider } = await import('@/hooks/useTheme');
  const page = render(
    <ThemeProvider>
      <span>Panel loaded</span>
    </ThemeProvider>,
  );
  expect(screen.getByText('Panel loaded')).toBeTruthy();
  page.unmount();
});
