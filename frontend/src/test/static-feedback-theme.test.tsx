import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import { ConfigProvider, Modal, message, notification, theme } from 'antd';

import { buildAntdThemeConfig, ThemeProvider, useTheme, type ThemeMode } from '@/hooks/useTheme';

const modes: ThemeMode[] = ['light', 'dark', 'ultra-dark', 'colorful', 'blue-gray', 'cyberpunk'];

function PaletteProbe() {
  const { token } = theme.useToken();
  return (
    <input aria-label="feedback palette" data-bg={token.colorBgElevated} defaultValue="keep me" />
  );
}

function Controls() {
  const { setThemeMode, toggleLowPower } = useTheme();
  return (
    <>
      {modes.map((mode) => (
        <button key={mode} onClick={() => setThemeMode(mode)}>
          {mode}
        </button>
      ))}
      <button onClick={toggleLowPower}>low power</button>
    </>
  );
}

afterEach(async () => {
  await act(async () => {
    Modal.destroyAll();
    message.destroy();
    notification.destroy();
  });
  ConfigProvider.config({ holderRender: undefined });
  localStorage.clear();
});

test('static dialogs, messages and notifications inherit each palette outside the app root', async () => {
  render(
    <ThemeProvider>
      <Controls />
    </ThemeProvider>,
  );
  await act(async () => {
    Modal.info({ title: 'Dialog', content: <PaletteProbe /> });
    message.info({ content: <PaletteProbe />, duration: 0 });
    notification.info({ title: 'Notification', description: <PaletteProbe />, duration: 0 });
  });
  for (const mode of modes) {
    fireEvent.click(screen.getByRole('button', { name: mode }));
    const expected = theme.getDesignToken(buildAntdThemeConfig(mode)).colorBgElevated;
    await waitFor(() => {
      const probes = screen.getAllByRole('textbox', { name: 'feedback palette' });
      expect(probes).toHaveLength(3);
      probes.forEach((probe) => expect(probe.getAttribute('data-bg')).toBe(expected));
    });
  }
});

test('an open static dialog follows theme and low-power changes without resetting its input', async () => {
  render(
    <ThemeProvider>
      <Controls />
    </ThemeProvider>,
  );
  fireEvent.click(screen.getByRole('button', { name: 'light' }));
  await act(async () => {
    Modal.confirm({ title: 'Dialog', content: <PaletteProbe /> });
  });
  const input = await screen.findByRole('textbox', { name: 'feedback palette' });
  fireEvent.change(input, { target: { value: 'edited' } });
  for (const mode of ['cyberpunk', 'blue-gray'] as const) {
    fireEvent.click(screen.getByRole('button', { name: mode }));
    await waitFor(() =>
      expect(input.getAttribute('data-bg')).toBe(
        theme.getDesignToken(buildAntdThemeConfig(mode)).colorBgElevated,
      ),
    );
    expect(screen.getByRole('textbox', { name: 'feedback palette' })).toBe(input);
    expect((input as HTMLInputElement).value).toBe('edited');
  }
  fireEvent.click(screen.getByRole('button', { name: 'low power' }));
  await waitFor(() =>
    expect(input.getAttribute('data-bg')).toBe(
      theme.getDesignToken(buildAntdThemeConfig('dark')).colorBgElevated,
    ),
  );
});
