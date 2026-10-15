import { actionFor, isValidModelAlias, type Action, type Workspace } from '../providers/provider-features.model';
import { ProviderProblem } from '../providers/provider.model';

export function speedSettingsError(w: Workspace, id: string, tier: string, variants: string[]): string {
  if (!['', 'priority', 'ultrafast'].includes(tier)) return '请选择有效的固定档位。';
  if (variants.length && (tier || /\((fast|ultrafast)\)$/.test(id))) return '只能从不固定档位的普通映射创建速度分身。';
  if (new Set(variants).size !== variants.length || variants.some(v => !['fast', 'ultrafast'].includes(v))) return '请选择有效的速度分身。';
  if (variants.some(v => !isValidModelAlias(`${id}(${v})`))) return '分身名称过长，请先缩短普通映射别名。';
  if (variants.some(v => w.models.some(m => m.id === `${id}(${v})`))) return '此供应商已有同名速度分身，请取消勾选；其他供应商的同名冲突由保存时校验。';
  return '';
}

/** Reuse the provider transaction, preserving the selected channel and model revision. */
export function speedSettingsAction(w: Workspace, id: string, tier: string, variants: string[]): Action {
  const m = w.models.find(m => m.id === id);
  if (w.deleted || !m || speedSettingsError(w, id, tier, variants)) throw new ProviderProblem('invalid');
  return actionFor({ id: w.provider_id, revision: w.revision }, 'model-put', {
    target: id, model: m.upstream, model_revision: m.revision, strategy: m.strategy,
    key_ids: [...m.key_ids], tools: m.tools, streaming: m.streaming,
    service_tier: tier, speed_variants: [...variants],
  });
}
