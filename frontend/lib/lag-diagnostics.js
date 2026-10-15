// Reports deliberately use an allowlist: never serialize API objects or log text.
export const LAG_SYMPTOMS = {
  blocks: "Delayed blocks or rubber-banding",
  chunks: "Slow chunk loading",
  fps: "Low FPS or stuttering",
};
const number = (value, unit) =>
  typeof value === "number" && Number.isFinite(value) && value >= 0
    ? `${Math.round(value * 10) / 10}${unit}`
    : "Unavailable";
const bytes = (value) => number(typeof value === "number" ? value / 1048576 : null, " MB");
export function diagnosticReport({
  server,
  ready,
  powerState,
  usage,
  usageError,
  symptom,
  capturedAt,
}) {
  const live = ready && !usageError && usage;
  const state = ["running", "offline", "starting", "stopping"].includes(powerState)
    ? powerState
    : "unknown";
  const software = ["PAPER", "FABRIC", "FORGE", "VANILLA", "PURPUR", "SPIGOT", "NEOFORGE"].includes(
    server.server_type,
  )
    ? server.server_type
    : "Other";
  return [
    "Blockyard diagnostic report",
    `Captured: ${capturedAt}`,
    "Uses the last dashboard reading; capture time is not the measurement time.",
    `Server reference: ${server.id}`,
    `Software: ${software}`,
    `Player-reported symptom: ${LAG_SYMPTOMS[symptom] || "Not specified"}`,
    `Live readings: ${live ? "Available (single snapshot)" : "Unavailable; no stale readings included"}`,
    `Observed process state: ${live ? state : "unavailable"}`,
    `CPU use: ${live ? number(usage.cpu_percent, "%") : "Unavailable"}`,
    `CPU allowance: ${number(server.cpu_limit, "%")}`,
    `Memory use: ${live ? bytes(usage.memory_bytes) : "Unavailable"}`,
    `Memory allowance: ${number(server.ram_mb, " MB")}`,
    `Disk use: ${live ? bytes(usage.disk_bytes) : "Unavailable"}`,
    `Disk allowance: ${number(server.disk_mb, " MB")}`,
    `Process uptime: ${live ? number(typeof usage.uptime_ms === "number" ? usage.uptime_ms / 1000 : null, " seconds") : "Unavailable"}`,
    "Minecraft tick rate: Not measured",
    "Player ping / packet loss: Not measured",
    "A running process and resource snapshot do not confirm smooth gameplay or identify the cause of lag.",
    "No logs, file contents, player names, addresses or credentials are included.",
  ].join("\n");
}
