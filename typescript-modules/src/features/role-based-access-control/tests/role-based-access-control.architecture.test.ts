import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

function walkTsFiles(dir: string): string[] {
  if (!fs.existsSync(dir)) return [];
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  const files: string[] = [];
  for (const entry of entries) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...walkTsFiles(full));
    } else if (entry.name.endsWith(".ts") || entry.name.endsWith(".tsx")) {
      files.push(full);
    }
  }
  return files;
}

test("v3.0 TypeScript architecture: zero cross-feature imports and no legacy api/rest routes", () => {
  const srcRoot = path.resolve(process.cwd(), "src");
  assert.equal(fs.existsSync(path.join(srcRoot, "api")), false, "legacy src/api/ directory must be removed");

  const featuresRoot = path.join(srcRoot, "features");
  for (const filePath of walkTsFiles(featuresRoot)) {
    const rel = path.relative(featuresRoot, filePath).replaceAll(path.sep, "/");
    const owner = rel.split("/")[0];
    const source = fs.readFileSync(filePath, "utf-8");
    const importMatches = source.matchAll(/from\s+["']([^"']+)["']/g);
    for (const match of importMatches) {
      const target = match[1];
      if (!target) continue;
      if (target.includes("features/")) {
        const idx = target.indexOf("features/");
        const importedFeature = target.slice(idx + "features/".length).split("/")[0];
        assert.equal(importedFeature, owner, `${rel} MUST NOT import feature ${importedFeature}`);
      }
    }
  }
});
