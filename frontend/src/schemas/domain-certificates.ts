import { z } from 'zod';

export const CertificateInventorySchema = z.object({
  items: z.array(
    z.object({
      id: z.string(),
      name: z.string(),
      names: z.array(z.string()),
      usedBy: z.array(z.string()),
      manager: z.string(),
      challenge: z.string(),
      ports: z.array(z.number().int()),
      source: z.string(),
      state: z.string(),
      issuer: z.string(),
      expiresAt: z.number().int(),
      daysLeft: z.number().int(),
      autoRenew: z.boolean(),
      certFile: z.string().optional(),
      warning: z.string().optional(),
      nodeId: z.number().int().optional(),
      nodeName: z.string().optional(),
    }),
  ),
  firewallEnabled: z.boolean(),
  firewallBackend: z.string(),
  firewallWarning: z.string().optional(),
});

export type CertificateInventory = z.infer<typeof CertificateInventorySchema>;
