// Core diagnostics sometimes contain HTML-encoded array indexes. Decode text,
// never render it as markup: notifications and React nodes escape it themselves.
export function readableMessage(value: string): string {
  return value.replace(/&#(?:x([0-9a-f]+)|([0-9]+));/gi, (original, hex, decimal) => {
    const code = Number.parseInt(hex || decimal, hex ? 16 : 10);
    return code >= 32 && code <= 0x10ffff && !(code >= 0xd800 && code <= 0xdfff)
      ? String.fromCodePoint(code)
      : original;
  });
}
