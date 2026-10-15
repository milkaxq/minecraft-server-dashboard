export function visibleFiles(entries, search, order) {
  const query = search.trim().toLowerCase();
  return entries
    .filter((entry) => entry.name.toLowerCase().includes(query))
    .sort((a, b) => {
      if (a.is_file !== b.is_file) return a.is_file ? 1 : -1;
      if (order === "size" && a.is_file && a.size !== b.size) return b.size - a.size;
      const compared = a.name.localeCompare(b.name, "en", { numeric: true, sensitivity: "base" });
      return order === "name-desc" ? -compared : compared;
    });
}

export function folderBreadcrumbs(directory) {
  const parts = directory.split("/").filter(Boolean);
  return [
    { name: "Server files", path: "/" },
    ...parts.map((name, index) => ({ name, path: "/" + parts.slice(0, index + 1).join("/") })),
  ];
}
