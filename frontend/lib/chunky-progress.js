// Parse only Chunky's own logger output, never player chat containing similar text.
// Unknown/localized formats stay unclassified instead of implying success.
export function parseChunkyProgress(line) {
  if (typeof line !== "string" || line.length > 8192) return null;
  // eslint-disable-next-line no-control-regex
  const text = line.replace(/\x1b\[[0-9;]*[a-zA-Z]/g, "").trim();
  const source = text.match(/^(?:\[[^\]\r\n]+ INFO\]: )?\[Chunky\] (.+)$/);
  if (!source) return null;
  const running = source[1].match(
    /^Task running for ([a-zA-Z0-9_-]{1,64})\. Processed: (\d+) chunks \((\d+(?:\.\d+)?)%\), ETA: (\d+:\d{2}:\d{2}), Rate: (\d+(?:\.\d+)?) cps(?:, Current: -?\d+, -?\d+)?$/,
  );
  if (!running) return null;
  const [, world, chunks, percent, eta, rate] = running;
  if (
    Number(percent) > 100 ||
    !Number.isSafeInteger(Number(chunks)) ||
    !Number.isFinite(Number(rate))
  )
    return null;
  return { world, chunks: Number(chunks), percent: Number(percent), eta, rate: Number(rate) };
}
