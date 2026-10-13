import { readdir, mkdir, copyFile, writeFile } from "node:fs/promises";
import path from "node:path";

const source = path.resolve("test-results");
const destination = path.resolve("../.local/design-review");
await mkdir(destination, { recursive: true });
const groups = { laptop: [], mobile: [], interactions: [] };
for (const directory of await readdir(source, { withFileTypes: true })) {
  if (
    !directory.isDirectory() ||
    !/visual-audit|custom-dropdown|catalog-shows-ac/.test(directory.name)
  )
    continue;
  for (const filename of await readdir(path.join(source, directory.name))) {
    if (!filename.endsWith(".png")) continue;
    const group = filename.startsWith("laptop")
      ? "laptop"
      : filename.startsWith("mobile")
        ? "mobile"
        : "interactions";
    await copyFile(path.join(source, directory.name, filename), path.join(destination, filename));
    groups[group].push(filename);
  }
}
const escape = (value) =>
  value.replaceAll("&", "&amp;").replaceAll('"', "&quot;").replaceAll("<", "&lt;");
const count = Object.values(groups).reduce((total, files) => total + files.length, 0);
if (!count)
  throw new Error(
    "No design screenshots found. Build the frontend and run npm run test:visual first.",
  );
await writeFile(
  path.join(destination, "index.html"),
  `<!doctype html>
<html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Blockyard design review</title>
<style>body{margin:0;background:#f4f6f3;color:#202c26;font:15px system-ui}main{max-width:1400px;margin:auto;padding:32px}h1{font-size:32px;letter-spacing:-1px}p{color:#58675e;line-height:1.7}nav{display:flex;gap:16px;margin:24px 0}a{color:#306b41}section{margin:36px 0}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(240px,1fr));gap:24px}figure{margin:0;background:white;border:1px solid #d8e1d9;border-radius:14px;overflow:hidden}img{width:100%;height:340px;object-fit:cover;object-position:top;display:block;border-bottom:1px solid #e4e9e3}figcaption{padding:16px;text-transform:capitalize;font-weight:600}h2{font-size:22px}@media(max-width:600px){main{padding:20px}.grid{grid-template-columns:1fr}}</style>
<main><h1>Blockyard design review</h1><p>${count} screenshots using test accounts and mocked APIs. Laptop: 1366 × 900. Mobile: 390 × 844. Click a preview to see the complete page.</p>
<nav><a href="#laptop">Laptop</a><a href="#mobile">Mobile</a><a href="#interactions">Dropdown & installation</a></nav>
${Object.entries(groups)
  .map(
    ([group, files]) =>
      `<section id="${group}"><h2>${group === "interactions" ? "Dropdown & installation" : group === "laptop" ? "Laptop" : "Mobile"}</h2><div class="grid">${files
        .sort()
        .map(
          (filename) =>
            `<figure><a href="${escape(filename)}"><img loading="lazy" src="${escape(filename)}" alt="${escape(filename.replaceAll("-", " "))}"><figcaption>${escape(
              filename
                .replace(/^(laptop|mobile)-*/, "")
                .replace(/\.png$/, "")
                .replaceAll("-", " "),
            )}</figcaption></a></figure>`,
        )
        .join("")}</div></section>`,
  )
  .join("")}
</main></html>`,
);
console.log(`${count} screenshots: ${path.join(destination, "index.html")}`);
