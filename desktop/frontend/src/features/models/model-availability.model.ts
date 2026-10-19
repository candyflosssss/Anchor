import { keyAvailability, type AvailabilityState, type SourceKey } from '../providers/channel-availability.model';
import type { ModelCandidate, ModelGroup } from '../providers/provider-summary.model';

/** Presentation only: never reorder the persisted routing configuration. */
export function splitUnavailable<T>(items: T[], state: (item: T) => AvailabilityState) {
  const visible: T[] = [], unavailable: T[] = [];
  for (const item of items) (state(item) === 'unavailable' ? unavailable : visible).push(item);
  return { visible, unavailable };
}

export function modelAvailability(group: ModelGroup): AvailabilityState {
  if (group.candidates.some(c => c.availability.state === 'available')) return 'available';
  // A partial or failed workspace read is never evidence that all sources are gone.
  if (!group.complete || group.candidates.some(c => c.availability.state === 'unknown')) return 'unknown';
  return 'unavailable';
}

export const visibleModels = (groups: ModelGroup[]) => splitUnavailable(groups, modelAvailability);
export const visibleCandidates = (candidates: ModelCandidate[]) => splitUnavailable(candidates, c => c.availability.state);
export const visibleKeys = (keys: SourceKey[]) => splitUnavailable(keys, k => keyAvailability(k).state);
