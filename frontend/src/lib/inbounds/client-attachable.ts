import { protocolCapabilities } from '@/generated/protocol-capabilities';

export function isClientAttachableProtocol(protocol: string | undefined): boolean {
  const capability =
    protocolCapabilities[(protocol || '').toLowerCase() as keyof typeof protocolCapabilities];
  return capability?.clientAttachable ?? false;
}
