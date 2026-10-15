// These labels are filename hints, not proof that a plugin is loaded or trusted.
export function installedAddonInfo(filename) {
  if (typeof filename !== "string") return { name: "Unknown addon", guide: null };
  if (filename.toLowerCase().includes("voicechat"))
    return { name: "Simple Voice Chat", guide: "voice" };
  if (/^chunky(?:[-_.]|\.jar$)/i.test(filename)) return { name: "Chunky", guide: "chunky" };
  return { name: filename, guide: null };
}

export function hasGuidedSetup(filename, permissions = []) {
  const { guide } = installedAddonInfo(filename);
  return guide === "voice"
    ? permissions.includes("settings")
    : guide === "chunky" && permissions.includes("console");
}

export function filterInstalledAddons(files, search, guidedOnly, permissions) {
  const query = search.trim().toLowerCase();
  return files.filter((file) => {
    const info = installedAddonInfo(file.name);
    return (
      `${info.name} ${file.name}`.toLowerCase().includes(query) &&
      (!guidedOnly || hasGuidedSetup(file.name, permissions))
    );
  });
}
