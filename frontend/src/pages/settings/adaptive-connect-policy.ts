export interface AdaptiveConnectPolicy {
  version: 1;
  enabled: boolean;
  failureThreshold: number;
  recoveryThreshold: number;
  cooldownSeconds: number;
  maxAgeSeconds: number;
  failback: boolean;
}

export const DEFAULT_ADAPTIVE_POLICY: Readonly<AdaptiveConnectPolicy> = Object.freeze({
  version: 1,
  enabled: false,
  failureThreshold: 2,
  recoveryThreshold: 3,
  cooldownSeconds: 30,
  maxAgeSeconds: 90,
  failback: false,
});

export function exportAdaptivePolicy(policy: AdaptiveConnectPolicy): string {
  const validInt = (n: number, min: number, max: number) =>
    Number.isInteger(n) && n >= min && n <= max;
  if (
    policy.version !== 1 ||
    typeof policy.enabled !== 'boolean' ||
    typeof policy.failback !== 'boolean' ||
    !validInt(policy.failureThreshold, 1, 10) ||
    !validInt(policy.recoveryThreshold, 1, 10) ||
    !validInt(policy.cooldownSeconds, 0, 300) ||
    !validInt(policy.maxAgeSeconds, 5, 600)
  ) {
    throw new Error('Invalid Adaptive Connect policy');
  }
  // Copy only the public v1 fields. Never include other form data, tokens,
  // inbound identifiers, subscription URLs or credentials.
  return (
    JSON.stringify(
      {
        version: policy.version,
        enabled: policy.enabled,
        failureThreshold: policy.failureThreshold,
        recoveryThreshold: policy.recoveryThreshold,
        cooldownSeconds: policy.cooldownSeconds,
        maxAgeSeconds: policy.maxAgeSeconds,
        failback: policy.failback,
      },
      null,
      2,
    ) + '\n'
  );
}
