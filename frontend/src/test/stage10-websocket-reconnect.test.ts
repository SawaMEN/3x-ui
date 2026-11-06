import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { WebSocketClient } from '@/api/websocket';

class FakeSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;

  readyState = FakeSocket.CONNECTING;
  readonly handlers = new Map<string, Set<() => void>>();
  constructor(public readonly url: string) {
    sockets.push(this);
  }
  addEventListener(name: string, handler: () => void) {
    if (!this.handlers.has(name)) this.handlers.set(name, new Set());
    this.handlers.get(name)!.add(handler);
  }
  emit(name: string) {
    if (name === 'open') this.readyState = FakeSocket.OPEN;
    if (name === 'close') this.readyState = FakeSocket.CLOSED;
    for (const handler of this.handlers.get(name) ?? []) handler();
  }
  send() {}
  close() {
    this.readyState = FakeSocket.CLOSED;
    this.emit('close');
  }
}

const sockets: FakeSocket[] = [];
let originalWS: typeof WebSocket;

beforeEach(() => {
  vi.useFakeTimers();
  sockets.length = 0;
  originalWS = globalThis.WebSocket;
  globalThis.WebSocket = FakeSocket as unknown as typeof WebSocket;
});

afterEach(() => {
  globalThis.WebSocket = originalWS;
  vi.useRealTimers();
});

describe('stage 10: reconnect after mobile foreground and offline', () => {
  it('opens exactly one replacement when connectivity returns and cancels the old backoff', () => {
    const client = new WebSocketClient('/');
    const onConnected = vi.fn();
    client.on('connected', onConnected);
    client.connect();
    client.connect();
    expect(sockets).toHaveLength(1);
    sockets[0].emit('open');
    expect(onConnected).toHaveBeenCalledTimes(1);
    sockets[0].emit('close');
    expect(client.isConnected).toBe(false);

    window.dispatchEvent(new Event('online'));
    window.dispatchEvent(new Event('online'));
    expect(sockets).toHaveLength(2);
    sockets[1].emit('open');
    expect(onConnected).toHaveBeenCalledTimes(2);
    vi.advanceTimersByTime(60_000);
    expect(sockets).toHaveLength(2);

    client.disconnect();
    window.dispatchEvent(new Event('online'));
    vi.advanceTimersByTime(60_000);
    expect(sockets).toHaveLength(2);
  });

  it('retries on foreground instead of waiting for an exponential reconnect timer', () => {
    const client = new WebSocketClient('/');
    client.connect();
    sockets[0].emit('close');
    document.dispatchEvent(new Event('visibilitychange'));
    expect(sockets).toHaveLength(2);
    client.disconnect();
  });

  it('keeps one listener per event across repeated connect and after disconnect', () => {
    const client = new WebSocketClient('/');
    client.connect();
    client.connect();
    sockets[0].emit('close');
    window.dispatchEvent(new Event('online'));
    expect(sockets).toHaveLength(2);
    client.disconnect();
    window.dispatchEvent(new Event('online'));
    expect(sockets).toHaveLength(2);
  });
});
