export function rootRedirectError(raw: string, domains: string[] = []): string {
  if ([...raw].some((char) => char.charCodeAt(0) < 32 || char.charCodeAt(0) === 127))
    return 'Адрес содержит управляющие символы.';
  const value = raw.trim();
  if (!value) return 'Укажите адрес перенаправления.';
  if (new TextEncoder().encode(value).length > 2048) return 'Адрес не должен превышать 2048 байт.';
  try {
    const target = new URL(value);
    if (!['https:', 'http:'].includes(target.protocol))
      return 'Используйте полный адрес http:// или https://.';
    if (target.username || target.password) return 'Логин и пароль в адресе запрещены.';
    if (
      (!target.pathname || target.pathname === '/') &&
      target.searchParams.get('panel') !== '1' &&
      domains.some(
        (name) =>
          name.toLowerCase().replace(/\.$/, '') ===
          target.hostname.toLowerCase().replace(/\.$/, ''),
      )
    )
      return 'Корень обслуживаемого домена не может вести сам на себя. Укажите другую страницу или сайт.';
    return '';
  } catch {
    return 'Некорректный адрес перенаправления.';
  }
}
