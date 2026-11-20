import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { WebSocketClient } from '@/api/websocket';

class FakeSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;

  readyState = FakeSocket.CONNECTING;
  readonly handlers = new Map<string, Set<(event?: { data: string }) => void>>();
  constructor(public readonly url: string) {
    sockets.push(this);
  }
  addEventListener(name: string, handler: (event?: { data: string }) => void) {
    if (!this.handlers.has(name)) this.handlers.set(name, new Set());
    this.handlers.get(name)!.add(handler);
  }
  emit(name: string, event?: { data: string }) {
    if (name === 'open') this.readyState = FakeSocket.OPEN;
    if (name === 'close') this.readyState = FakeSocket.CLOSED;
    for (const handler of this.handlers.get(name) ?? []) handler(event);
  }
  send() {}
  close(code = 1000) {
    if (code !== 1000 && (code < 3000 || code > 4999)) {
      throw new DOMException('Invalid WebSocket close code', 'InvalidAccessError');
    }
    this.readyState = FakeSocket.CLOSED;
    this.emit('close');
  }
}

const sockets: FakeSocket[] = [];

beforeEach(() => {
  vi.useFakeTimers();
  sockets.length = 0;
  const browser = new EventTarget();
  const doc = new EventTarget();
  vi.stubGlobal('window', {
    location: { protocol: 'https:', host: 'panel.example' },
    addEventListener: browser.addEventListener.bind(browser),
    removeEventListener: browser.removeEventListener.bind(browser),
    dispatchEvent: browser.dispatchEvent.bind(browser),
  });
  vi.stubGlobal('document', {
    visibilityState: 'visible',
    addEventListener: doc.addEventListener.bind(doc),
    removeEventListener: doc.removeEventListener.bind(doc),
    dispatchEvent: doc.dispatchEvent.bind(doc),
  });
  vi.stubGlobal('WebSocket', FakeSocket);
});

afterEach(() => {
  vi.unstubAllGlobals();
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

  it('avoids a second socket when a disconnect handler reconnects immediately', () => {
    const client = new WebSocketClient('/');
    client.on('disconnected', () => client.connect());
    client.connect();
    sockets[0].emit('close');
    vi.advanceTimersByTime(60_000);
    expect(sockets).toHaveLength(2);
  });

  it('reports disconnected while a closing connection is replaced', () => {
    const client = new WebSocketClient('/');
    client.connect();
    sockets[0].emit('open');
    sockets[0].readyState = FakeSocket.CLOSING;
    client.connect();
    expect(sockets).toHaveLength(2);
    expect(client.isConnected).toBe(false);
  });

  it('closes an oversized message with a code that browsers permit', () => {
    const client = new WebSocketClient('/');
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      client.connect();
      sockets[0].emit('open');
      sockets[0].emit('message', { data: 'a'.repeat(10 * 1024 * 1024 + 1) });
      expect(sockets[0].readyState).toBe(FakeSocket.CLOSED);
      expect(client.isConnected).toBe(false);
    } finally {
      error.mockRestore();
    }
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
