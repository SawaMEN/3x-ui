import { afterEach, describe, expect, it, vi } from 'vitest';
import { isChunkLoadError } from '@/lib/chunk-load-recovery';

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('chunk loading recovery', () => {
  it('recognizes CSS preload failures as well as missing JS chunks', () => {
    expect(isChunkLoadError(new Error('Unable to preload CSS for /assets/ui-old.css'))).toBe(true);
    expect(
      isChunkLoadError(
        new TypeError('Failed to fetch dynamically imported module: /assets/page.js'),
      ),
    ).toBe(true);
    expect(
      isChunkLoadError(new TypeError("Cannot read properties of undefined (reading 'default')")),
    ).toBe(false);
  });

  it('cache busts a CSS failure once and preserves the current route', async () => {
    vi.resetModules();
    const replace = vi.fn();
    const values = new Map<string, string>();
    vi.stubGlobal('sessionStorage', {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
    });
    vi.stubGlobal('window', {
      location: { href: 'https://panel.example/secret/panel/inbounds?filter=1#clients', replace },
    });
    const recovery = await import('@/lib/chunk-load-recovery');
    expect(recovery.reloadAfterChunkLoadError()).toBe(true);
    expect(recovery.reloadAfterChunkLoadError()).toBe(true);
    expect(replace).toHaveBeenCalledTimes(1);
    const target = new URL(replace.mock.calls[0][0]);
    expect(target.pathname).toBe('/secret/panel/inbounds');
    expect(target.searchParams.get('filter')).toBe('1');
    expect(target.searchParams.has('_3xui_chunk_reload')).toBe(true);
    expect(target.hash).toBe('#clients');
    vi.resetModules();
    expect((await import('@/lib/chunk-load-recovery')).reloadAfterChunkLoadError()).toBe(false);
    expect(replace).toHaveBeenCalledTimes(1);
  });

  it('reloads on an undefined lazy import even without a preloadError event', async () => {
    vi.useFakeTimers();
    vi.resetModules();
    const replace = vi.fn();
    vi.stubGlobal('sessionStorage', { getItem: () => null, setItem: vi.fn() });
    vi.stubGlobal('window', {
      location: { href: 'https://panel.example/secret/panel/clients', replace },
    });
    const recovery = await import('@/lib/chunk-load-recovery');
    const result = recovery.importWithChunkRecovery(async () => undefined);
    await Promise.resolve();
    expect(replace).toHaveBeenCalledTimes(1);
    const assertion = expect(result).rejects.toThrow(/could not finish reloading/);
    await vi.advanceTimersByTimeAsync(8_000);
    await assertion;
  });
  it('temporarily suspends React.lazy while reloading, but never forever', async () => {
    vi.useFakeTimers();
    vi.resetModules();
    vi.stubGlobal('sessionStorage', { getItem: () => null, setItem: vi.fn() });
    vi.stubGlobal('window', {
      location: { href: 'https://panel.example/secret/panel/', replace: vi.fn() },
    });
    const recovery = await import('@/lib/chunk-load-recovery');
    expect(recovery.reloadAfterChunkLoadError()).toBe(true);
    let settled = false;
    const result = recovery.importWithChunkRecovery(async () => undefined);
    const completed = result.then(
      () => {
        settled = true;
      },
      () => {
        settled = true;
      },
    );
    await Promise.resolve();
    await Promise.resolve();
    expect(settled).toBe(false);
    const assertion = expect(result).rejects.toThrow(/could not finish reloading/);
    await vi.advanceTimersByTimeAsync(8_000);
    await assertion;
    await completed;
    expect(settled).toBe(true);
  });
});
