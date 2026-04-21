#!/usr/bin/env node
// Sync docs/specifications/ui-explanations/<module>.md into
// ui-public/src/i18n/en.json. See the README in that docs folder for
// the authoring format and rationale.
//
// Usage:
//   node tools/i18n/sync-ui-explanations.mjs
//   node tools/i18n/sync-ui-explanations.mjs --check   # exit 1 if en.json is stale

import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "..", "..");
const sourceDir = resolve(repoRoot, "docs/specifications/ui-explanations");
const enJsonPath = resolve(repoRoot, "ui-public/src/i18n/en.json");

const KEY_PREFIX_TC_DESC = "pub.tc_desc.";
const KEY_PREFIX_TAG = "pub.tag.";

function parseModuleFile(text, moduleName) {
  // The parser is a small state machine. It walks the file line by
  // line and recognises three section headers:
  //   "## Testcase <id>"
  //   "## Tag <TAG>"
  //   "## ..."   (anything else ends the current section)
  // Inside a section it looks for "Header:" and "Description:" labels.
  // Body text between the "Description:" label and the next section
  // header (or EOF) is collected as the description.
  const out = {};
  const lines = text.split(/\r?\n/);
  let mode = null;        // "testcase" | "tag" | null
  let id = null;          // current testcase id or tag name
  let buffer = [];        // lines collected for the current Description:
  let capturing = false;  // true once a "Description:" label has been seen

  const flushDescription = () => {
    if (!capturing || id === null) return;
    const text = buffer.join("\n").trim();
    if (!text) return;
    if (mode === "testcase") {
      out[`${KEY_PREFIX_TC_DESC}${id.toLowerCase()}`] = text;
    } else if (mode === "tag") {
      out[`${KEY_PREFIX_TAG}${moduleName.toLowerCase()}.${id}.desc`] = text;
    }
  };

  const resetSection = () => {
    flushDescription();
    mode = null;
    id = null;
    buffer = [];
    capturing = false;
  };

  for (const raw of lines) {
    const line = raw.trimEnd();

    const tcMatch = line.match(/^##\s+Testcase\s+(\S+)\s*$/);
    if (tcMatch) {
      resetSection();
      mode = "testcase";
      id = tcMatch[1];
      continue;
    }
    const tagMatch = line.match(/^##\s+Tag\s+(\S+)\s*$/);
    if (tagMatch) {
      resetSection();
      mode = "tag";
      id = tagMatch[1];
      continue;
    }
    if (line.startsWith("## ")) {
      // Some other section header (e.g. document title continuation).
      resetSection();
      continue;
    }

    if (mode === "tag") {
      const header = line.match(/^Header:\s*(.+?)\s*$/);
      if (header) {
        out[`${KEY_PREFIX_TAG}${moduleName.toLowerCase()}.${id}.header`] = header[1];
        continue;
      }
    }

    if ((mode === "testcase" || mode === "tag") && /^Description:\s*$/.test(line)) {
      capturing = true;
      buffer = [];
      continue;
    }

    if (capturing) {
      buffer.push(raw);
    }
  }
  resetSection();

  return out;
}

function collectManagedKeys() {
  const files = readdirSync(sourceDir)
    .filter((f) => f.endsWith(".md") && f !== "README.md")
    .sort();
  const merged = {};
  for (const file of files) {
    const moduleName = file.replace(/\.md$/, "");
    const text = readFileSync(resolve(sourceDir, file), "utf8");
    const keys = parseModuleFile(text, moduleName);
    for (const [k, v] of Object.entries(keys)) {
      if (merged[k] !== undefined) {
        throw new Error(`duplicate key across modules: ${k}`);
      }
      merged[k] = v;
    }
  }
  return merged;
}

// Preserve insertion order of existing keys; append new keys at the end
// grouped by family for readability.
function mergeIntoCatalog(catalog, managed) {
  const out = { ...catalog };
  const existingKeys = new Set(Object.keys(out));
  for (const [k, v] of Object.entries(managed)) {
    out[k] = v;
  }
  const added = [];
  const updated = [];
  for (const k of Object.keys(managed)) {
    if (existingKeys.has(k)) {
      if (catalog[k] !== managed[k]) updated.push(k);
    } else {
      added.push(k);
    }
  }
  return { merged: out, added, updated };
}

function isManagedKey(k) {
  return k.startsWith(KEY_PREFIX_TC_DESC) || k.startsWith(KEY_PREFIX_TAG);
}

function removedKeys(catalog, managed) {
  // Keys that used to be managed but were dropped from the markdown.
  // Stay defensive: only flag orphans inside our own namespaces.
  const keep = new Set(Object.keys(managed));
  return Object.keys(catalog).filter((k) => isManagedKey(k) && !keep.has(k));
}

function serialize(catalog) {
  // 2-space indentation + trailing newline, matching the existing files.
  return JSON.stringify(catalog, null, 2) + "\n";
}

function main() {
  const check = process.argv.includes("--check");
  const managed = collectManagedKeys();
  const catalog = JSON.parse(readFileSync(enJsonPath, "utf8"));
  const { merged, added, updated } = mergeIntoCatalog(catalog, managed);
  const orphans = removedKeys(catalog, managed);

  const next = serialize(merged);
  const current = readFileSync(enJsonPath, "utf8");

  if (check) {
    if (next !== current) {
      console.error(
        `ui-public/src/i18n/en.json is out of sync with ${sourceDir}. ` +
          `Run: node tools/i18n/sync-ui-explanations.mjs`,
      );
      process.exit(1);
    }
    console.log("en.json is up to date.");
    return;
  }

  writeFileSync(enJsonPath, next);
  console.log(`Synced ${Object.keys(managed).length} managed keys into en.json.`);
  if (added.length) console.log(`  added:   ${added.length}`);
  if (updated.length) console.log(`  updated: ${updated.length}`);
  if (orphans.length) {
    console.log(`  orphans (in en.json but not in markdown, left untouched):`);
    for (const k of orphans) console.log(`    ${k}`);
  }
}

main();
