export const uniqueTitle = (prefix: string) =>
  `${prefix} ${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;

export const uniquePrompt = (prefix: string) => uniqueTitle(prefix);
