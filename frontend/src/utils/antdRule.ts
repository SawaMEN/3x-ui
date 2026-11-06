import type { z } from 'zod';
import type { TFunction } from 'i18next';

export function antdRule(schema: z.ZodType, t: TFunction) {
  return {
    validator: async (_rule: unknown, value: unknown) => {
      const result = schema.safeParse(value);
      if (!result.success) throw new Error(t(result.error.issues[0].message));
    },
  };
}
