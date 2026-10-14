/** User-facing names shared by providers, Keys, model aliases and connections. */
export const identifierPattern = String.raw`[\p{L}\p{Nd}][\p{L}\p{Nd}._\-]{0,127}`;
const identifier = new RegExp(String.raw`^${identifierPattern}(?![\s\S])`, 'u');
export const isValidId = (value: string) => identifier.test(value);
