import { afterEach, expect, it, vi } from 'vitest';
import { act, fireEvent, renderHook, screen } from '@testing-library/react';
import { QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import HeaderMapEditor from '@/components/form/HeaderMapEditor';
import { useClients } from '@/hooks/useClients';
import { HttpUtil, Msg } from '@/utils';
import { makeTestQueryClient, renderWithProviders } from './test-utils';
import { formatInboundTag, buildRemarkByTag } from '@/pages/xray/routing/helpers';

afterEach(() => vi.restoreAllMocks());

it.each(['constructor', '__proto__', 'toString'])(
  'edits the legal HTTP header %s without losing it',
  (name) => {
    const onChange = vi.fn();
    const page = renderWithProviders(
      <HeaderMapEditor
        mode="v2"
        value={Object.fromEntries([[name, ['old']]])}
        onChange={onChange}
      />,
    );
    fireEvent.change(screen.getByDisplayValue('old'), { target: { value: 'new' } });
    expect(JSON.stringify(onChange.mock.lastCall?.[0])).toBe(
      JSON.stringify(Object.fromEntries([[name, ['new']]])),
    );
    page.unmount();
  },
);

it('renders inbound tag names that collide with built-in object members', () => {
  expect(formatInboundTag('constructor')).toBe('constructor');
  expect(
    formatInboundTag(
      '__proto__',
      buildRemarkByTag([{ tag: '__proto__', remark: 'Imported rule' }]),
    ),
  ).toBe('__proto__ (Imported rule)');
});

it('retains live traffic for the client named __proto__', () => {
  vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', {}));
  const client = makeTestQueryClient();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const { result, unmount } = renderHook(() => useClients({ list: false }), { wrapper });
  act(() =>
    result.current.applyTrafficEvent({
      clientTraffics: [{ email: '__proto__', up: 100, down: 50 }],
    }),
  );
  expect(Object.keys(result.current.clientSpeed)).toContain('__proto__');
  expect(result.current.clientSpeed['__proto__']).toEqual({ up: 20, down: 10 });
  unmount();
});
