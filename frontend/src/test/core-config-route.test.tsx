import { render, screen } from '@testing-library/react';
import { it, expect, vi } from 'vitest';
import CoreConfigRoutePage from '@/pages/CoreConfigRoutePage';
vi.mock('@/pages/singbox/SingBoxRoutePage', () => ({ default: () => <div>sing-box editor</div> }));
it('shared menu paths always open the owner core editor', () => {
  render(<CoreConfigRoutePage />);
  expect(screen.getByText('sing-box editor')).toBeTruthy();
});
