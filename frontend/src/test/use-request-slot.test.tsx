import { act, renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { useRequestSlot } from '@/hooks/useRequestSlot';

describe('useRequestSlot', () => {
  it('cancels a superseded request without clearing its successor', () => {
    const { result } = renderHook(useRequestSlot);
    const first = result.current.begin();
    const second = result.current.begin();
    expect(first.signal.aborted).toBe(true);
    expect(result.current.finish(first)).toBe(false);
    expect(result.current.isPending()).toBe(true);
    expect(result.current.finish(second)).toBe(true);
    expect(result.current.isPending()).toBe(false);
  });

  it('aborts pending work when the viewer unmounts', () => {
    const { result, unmount } = renderHook(useRequestSlot);
    const request = result.current.begin();
    act(() => unmount());
    expect(request.signal.aborted).toBe(true);
    expect(result.current.finish(request)).toBe(false);
  });
});
