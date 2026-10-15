// Literal, case-insensitive search. Keep indexes in the original UTF-16 string so
// textarea selections remain correct with Unicode text. Bound match storage.
export function findInFile(text, query, limit = 1000) {
  const matches = [];
  if (!query || query.length > 200) return { matches, truncated: false };
  const escaped = query.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  for (const match of text.matchAll(new RegExp(escaped, "giu"))) {
    if (matches.length === limit) return { matches, truncated: true };
    matches.push({ start: match.index, end: match.index + match[0].length });
  }
  return { matches, truncated: false };
}
