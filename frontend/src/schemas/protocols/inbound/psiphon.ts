import { z } from 'zod';

export const PsiphonInboundSettingsSchema = z.object({
  serverAddress: z.string().default(''),
  tunnelProtocol: z.enum(['OSSH', 'SSH', 'TLS-OSSH', 'QUIC-OSSH']).default('OSSH'),
  serverEntry: z.string().default(''),
  additionalArguments: z.array(z.string()).default([]),
});
export type PsiphonInboundSettings = z.infer<typeof PsiphonInboundSettingsSchema>;
