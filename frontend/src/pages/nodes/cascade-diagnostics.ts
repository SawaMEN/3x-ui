/** Cascade diagnostics are evidence-based: a listening port is not a working VPN route. */
export type EvidenceStatus = 'ok' | 'failed' | 'warning' | 'unknown';
export type ProblemStatus = 'open' | 'fixed' | 'verified' | 'retest';
export type ProblemKind = 'connectivity' | 'authentication' | 'routing' | 'configuration' | 'other';

export interface CascadeTarget {
  tag: string;
  inboundId: number;
  nodeId: number;
  nodeName: string;
  remark: string;
  protocol: string;
  port: number;
  portLabel?: string;
  available: boolean;
  reason?: string;
}
export interface CascadeProbe {
  tag: string;
  reachable: boolean;
  checkedTarget: boolean;
  transport: string;
  latencyMs?: number;
  tunnelChecked?: boolean;
  inconclusive?: boolean;
  detail: string;
}
export interface RouteResult {
  matched: boolean;
  outboundTag: string;
  groupTags?: string[];
}
export interface CascadeCheck {
  at: string;
  tag: string;
  sourceInbound: string;
  probe?: CascadeProbe;
  probeError?: string;
  route?: RouteResult;
  routeError?: string;
  note?: string;
}
export interface CascadeProblem {
  id: string;
  tag: string;
  nodeId: number;
  protocol: string;
  type: ProblemKind;
  title: string;
  status: ProblemStatus;
  evidence?: string;
  updatedAt: string;
  automatic?: boolean;
}
export interface Stage {
  id: string;
  name: string;
  expected: string;
  actual: string;
  status: EvidenceStatus;
}
export const DIAGNOSTIC_CHECKLIST = [
  'Нода доступна, запущено нужное ядро и слушает целевой порт.',
  'Удалённое входящее подключение включено и поддерживается ядром источника.',
  'Служебные учётные данные каскада созданы и доставлены на удалённую ноду.',
  'Исходящее node-route-* создано в активной конфигурации, нет blackhole/block.',
  'Маршрутизатор выбирает ожидаемый тег для локального входящего.',
  'Аутентифицированный HTTP-запрос через служебное исходящее проходит.',
  'Трафик реального клиента прошёл из локального входящего через удалённую ноду.',
  'После исправления выполнена повторная проверка и приложены её результаты.',
] as const;

export function stages(target: CascadeTarget, check?: CascadeCheck): Stage[] {
  const probe = check?.probe;
  const route = check?.route;
  const mapped = check?.sourceInbound && check?.tag === target.tag;
  return [
    {
      id: 'target',
      name: 'Удалённая нода и входящее',
      expected: 'Нода онлайн, входящее доступно',
      actual: target.available ? 'Доступно для подготовки' : target.reason || 'Недоступно',
      status: target.available ? 'ok' : 'failed',
    },
    {
      id: 'listener',
      name: 'Транспорт / доступность порта',
      expected: 'Нода принимает TCP/UDP-пакеты',
      actual: check?.probeError || probe?.detail || 'Не проверено',
      // An authenticated HTTP probe proves transport, otherwise the probe only tests a socket.
      status: check?.probeError
        ? 'failed'
        : !probe
          ? 'unknown'
          : probe.reachable
            ? 'ok'
            : probe.inconclusive
              ? 'warning'
              : 'failed',
    },
    {
      id: 'auth',
      name: 'Учётные данные и исходящий туннель',
      expected: 'Аутентифицированный HTTP-запрос через каскад успешен',
      actual: !probe
        ? 'Не проверено'
        : probe.tunnelChecked
          ? probe.reachable
            ? 'Аутентифицированный HTTP-запрос успешен'
            : probe.detail
          : 'Проверен только транспорт, авторизация не подтверждена',
      status: !probe
        ? 'unknown'
        : !probe.tunnelChecked
          ? 'warning'
          : probe.reachable
            ? 'ok'
            : 'failed',
    },
    {
      id: 'route',
      name: 'Выбор маршрута локальным ядром',
      expected: 'Исходящий тег ' + target.tag,
      actual:
        check?.routeError ||
        (route
          ? route.matched
            ? route.outboundTag
            : 'Правило не совпало; выбран маршрут по умолчанию'
          : 'Не проверено'),
      status: check?.routeError
        ? 'warning'
        : !route
          ? 'unknown'
          : route.matched && route.outboundTag === target.tag && mapped
            ? 'ok'
            : 'failed',
    },
    {
      id: 'e2e',
      name: 'Реальный клиент → нода → интернет',
      expected: 'Сквозной трафик реального клиента подтверждён',
      actual: 'Требуется независимый тест клиентского входящего и логов обеих нод',
      status: 'unknown',
    },
  ];
}

export function verifiedForMerge(
  targets: CascadeTarget[],
  checks: Record<string, CascadeCheck>,
  problems: CascadeProblem[],
  liveEvidence: string,
  ciApproved: boolean,
): boolean {
  if (!targets.length || !liveEvidence.trim() || !ciApproved) return false;
  if (problems.some((p) => p.status !== 'verified')) return false;
  return targets.every((t) => {
    const s = stages(t, checks[t.tag]);
    return s.slice(0, 4).every((stage) => stage.status === 'ok');
  });
}

export function reconcileProbeProblem(
  previous: CascadeProblem[],
  target: CascadeTarget,
  check: CascadeCheck,
): CascadeProblem[] {
  const id = 'probe:' + target.tag;
  const bad = !!check.probeError || !check.probe?.reachable || !check.probe?.tunnelChecked;
  const old = previous.find((p) => p.id === id);
  if (!bad && !old) return previous;
  const timestamp = check.at;
  const status: ProblemStatus = bad ? 'open' : 'verified';
  const row: CascadeProblem = {
    id,
    tag: target.tag,
    nodeId: target.nodeId,
    protocol: target.protocol,
    type: check.probe?.tunnelChecked ? 'authentication' : 'connectivity',
    title: bad
      ? 'Не подтверждён аутентифицированный туннель'
      : 'Аутентифицированный туннель проверен',
    status,
    automatic: true,
    evidence: check.probeError || check.probe?.detail || '',
    updatedAt: timestamp,
  };
  return [...previous.filter((p) => p.id !== id), row];
}
