export interface HealthProps {
  status?: string;
  xrayState?: string;
  xrayError?: string;
  singboxState?: string;
  singboxError?: string;
  coreState?: string;
  coreError?: string;
  coreType?: string;
  runningCore?: string;
}

export function normalizeCoreName(value?: string): string {
  const core = (value || '').toLowerCase().trim().replaceAll('-', '');
  if (core === 'hiddifycore') return 'singbox';
  return core === 'xray' || core === 'singbox' ? core : '';
}

export function coreHealth(record: HealthProps) {
  const running = normalizeCoreName(record.runningCore);
  // Old node builds may know only the running core. Prefer an explicit
  // configured core when present, then fall back to the observed runtime.
  const configured = normalizeCoreName(record.coreType) || running || 'xray';
  const state =
    record.coreState ||
    (configured === 'singbox'
      ? record.singboxState
      : configured === 'xray'
        ? record.xrayState
        : '');
  const error =
    record.coreError ||
    (configured === 'singbox'
      ? record.singboxError
      : configured === 'xray'
        ? record.xrayError
        : '');
  return {
    configured,
    running,
    state: (state || '').toLowerCase().trim(),
    error: error || '',
  };
}

export function hasCoreProblem(record: HealthProps): boolean {
  if (record.status !== 'online') return false;
  const { configured, running, state } = coreHealth(record);
  if (running) return running !== configured;
  return state === 'error' || state === 'stop';
}
