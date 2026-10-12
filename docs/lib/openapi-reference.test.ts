import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

import { expect, it } from 'vitest';

it('every generated API page references an existing endpoint and method', () => {
  const spec = JSON.parse(readFileSync('./public/openapi.json', 'utf8')) as {
    paths: Record<string, Record<string, unknown>>;
  };
  function check(directory: string): void {
    for (const entry of readdirSync(directory, { withFileTypes: true })) {
      const file = join(directory, entry.name);
      if (entry.isDirectory()) {
        check(file);
      } else if (entry.name.endsWith('.mdx')) {
        const source = readFileSync(file, 'utf8');
        for (const match of source.matchAll(/operations=\{(\[.*?\])\}/g)) {
          const operations = JSON.parse(match[1]) as { path: string; method: string }[];
          for (const operation of operations) {
            expect(
              spec.paths[operation.path]?.[operation.method],
              `${file}: ${operation.method} ${operation.path}`,
            ).toBeDefined();
          }
        }
      }
    }
  }
  check('./content/docs');
});
