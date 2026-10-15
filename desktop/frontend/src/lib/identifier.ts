/** User-facing names shared by providers, Keys, model aliases and connections. */
export const identifierPattern = String.raw`[\p{L}\p{Nd}][\p{L}\p{Nd}._\-]{0,127}`;
const identifier = new RegExp(String.raw`^${identifierPattern}(?![\s\S])`, 'u');
export const isValidId = (value: string) => identifier.test(value);

/** Reserved suffixes apply only to model aliases, not provider/Key/connection IDs. */
export function isValidModelId(value: string): boolean {
  if (isValidId(value)) return true;
  const match = /^(.*)\((fast|ultrafast)\)$/.exec(value);
  return !!match && [...value].length <= 128 && isValidId(match[1]);
}
