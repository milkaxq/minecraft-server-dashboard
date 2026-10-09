export const SOFTWARE = {
  VANILLA: {
    name: "Vanilla",
    description: "Original Minecraft, without plugins or mods.",
    category: "classic",
  },
  PAPER: {
    name: "Paper",
    description: "A practical default for survival servers with plugins.",
    category: "plugins",
  },
  SPIGOT: {
    name: "Spigot",
    description: "Bukkit/Spigot plugins and traditional server behavior.",
    category: "plugins",
  },
  PURPUR: {
    name: "Purpur",
    description: "Paper-compatible plugins with extra gameplay settings.",
    category: "plugins",
  },
  FABRIC: {
    name: "Fabric",
    description: "Lightweight mods made specifically for Fabric.",
    category: "mods",
  },
  FORGE: {
    name: "Forge",
    description: "Forge mods and modpacks; match the pack’s loader version.",
    category: "mods",
  },
  NEOFORGE: {
    name: "NeoForge",
    description: "NeoForge mods and modpacks; separate from Forge.",
    category: "mods",
  },
};
export function softwareName(type) {
  return SOFTWARE[type]?.name || type;
}
export function supportsPlugins(type) {
  return SOFTWARE[type]?.category === "plugins";
}
export function supportsMods(type) {
  return SOFTWARE[type]?.category === "mods";
}
