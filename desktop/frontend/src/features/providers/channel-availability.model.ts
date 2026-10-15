import type { KeyStatus } from '../../desktop/contract.generated';

export type AvailabilityState = 'available' | 'unavailable' | 'unknown';
export interface ChannelAvailability { state: AvailabilityState; label: string; reason: string }
export interface SourceKey { id: string; key?: KeyStatus; anonymous: boolean }

/** Include disabled and missing scoped Keys: hiding them would imply anonymous access. */
export function channelKeys(scope: string[], keys: KeyStatus[]): SourceKey[] {
  if (!scope.length) return keys.length ? keys.map(key => ({ id: key.id, key, anonymous: false })) : [{ id: 'anonymous', anonymous: true }];
  return scope.map(id => {
    const key = keys.find(k => k.id === id);
    return { id, key, anonymous: !key && id === 'anonymous' && keys.length === 0 };
  });
}

export function keyAvailability(source: SourceKey): ChannelAvailability {
  if (source.anonymous) return { state: 'available', label: '匿名来源', reason: '未配置 Key，允许匿名请求；不代表上游无需认证。' };
  const k = source.key;
  if (!k) return { state: 'unavailable', label: 'Key 不存在', reason: '映射限定的 Key 已删除或不存在。' };
  if (!k.enabled || k.state === 'disabled') return { state: 'unavailable', label: '已停用', reason: 'Key 开关已关闭。' };
  if (k.state === 'ready') return { state: 'available', label: '当前可用', reason: 'Key 已启用且未处于冷却或认证失效状态。' };
  if (k.state === 'invalid') return { state: 'unavailable', label: '认证失效', reason: 'Key 被上游拒绝，需替换或恢复。' };
  if (k.state === 'cooling') return { state: 'unavailable', label: '冷却中', reason: 'Key 暂处于冷却状态，请刷新配置核对恢复情况。' };
  // The core's half-open snapshot does not distinguish a pending probe from an
  // in-flight probe. Do not promise availability or guess from a client clock.
  return { state: 'unknown', label: k.state === 'half-open' ? '恢复探测中' : '状态未知', reason: '当前快照不能确认 Key 可调度，请刷新配置核对。' };
}

export function channelAvailability(enabled: boolean, scope: string[], keys: KeyStatus[]): ChannelAvailability {
  if (!enabled) return { state: 'unavailable', label: '当前不可用', reason: '供应商已停用，不参与路由；Key 和映射配置保留。' };
  const states = channelKeys(scope, keys).map(keyAvailability);
  if (states.some(s => s.state === 'available')) return { state: 'available', label: '当前可用', reason: keys.length ? '至少一个来源 Key 当前可调度。' : '允许匿名请求；不代表上游无需认证。' };
  if (states.some(s => s.state === 'unknown')) return { state: 'unknown', label: '可用性待确认', reason: '来源 Key 正在恢复探测或状态未知，请刷新配置核对。' };
  return { state: 'unavailable', label: '当前不可用', reason: `无有效来源 Key：${[...new Set(states.map(s => s.label))].join('、')}。` };
}
