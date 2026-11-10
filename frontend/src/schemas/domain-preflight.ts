import { z } from 'zod';

export const DomainPreflightReportSchema = z.object({
  ready: z.boolean(),
  checkedAt: z.number().int(),
  checks: z.array(
    z.object({
      code: z.string(),
      severity: z.enum(['error', 'warning', 'info']),
      domain: z.string().optional(),
      message: z.string(),
      addresses: z.array(z.string()).optional(),
    }),
  ),
});

export type DomainPreflightReport = z.infer<typeof DomainPreflightReportSchema>;
