// End-to-end test for tools/i18n/sync-ui-explanations.mjs. The sync
// script lives outside ui-public (it writes into ui-public/src/i18n/)
// but its correctness is load-bearing for every authored explanation
// key, so we exercise it from here where a vitest runner is already
// configured. The test patches the absolute paths inside the script and
// runs it against a synthetic fixture in a tmpdir.

import { describe, expect, it } from "vitest";
import { execFileSync } from "node:child_process";
import {
  mkdtempSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
  rmSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "..", "..", "..");
const scriptPath = resolve(repoRoot, "tools/i18n/sync-ui-explanations.mjs");

function runSync(sourceDir, enJsonPath, args = []) {
  const src = readFileSync(scriptPath, "utf8");
  const patched = src
    .replace(
      /const sourceDir = [^;]+;/,
      `const sourceDir = ${JSON.stringify(sourceDir)};`,
    )
    .replace(
      /const enJsonPath = [^;]+;/,
      `const enJsonPath = ${JSON.stringify(enJsonPath)};`,
    );
  const patchedPath = resolve(dirname(enJsonPath), "sync-patched.mjs");
  writeFileSync(patchedPath, patched);
  // Pipe stderr rather than inheriting it: the staleness test below expects
  // the script to fail, and an inherited stderr makes that expected failure
  // look like a real build warning in the test output.
  return execFileSync("node", [patchedPath, ...args], {
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
}

function setupFixture() {
  const dir = mkdtempSync(resolve(tmpdir(), "gonemaster-sync-test-"));
  const sourceDir = resolve(dir, "ui-explanations");
  mkdirSync(sourceDir);
  const enJsonPath = resolve(dir, "en.json");
  return { dir, sourceDir, enJsonPath };
}

describe("sync-ui-explanations", () => {
  it("extracts testcase and tag keys from a well-formed markdown file", () => {
    const { dir, sourceDir, enJsonPath } = setupFixture();
    try {
      writeFileSync(
        resolve(sourceDir, "delegation.md"),
        [
          "# Public UI explanations: DELEGATION",
          "",
          "## Testcase delegation01",
          "",
          "Description:",
          "",
          "Two or more name servers improve resilience.",
          "",
          "## Tag NOT_ENOUGH_NS_DEL",
          "",
          "Header: Not enough nameservers",
          "",
          "Description:",
          "",
          "Too few servers makes the domain fragile.",
          "",
        ].join("\n"),
      );
      writeFileSync(
        enJsonPath,
        JSON.stringify({ "pub.app_title": "x" }, null, 2) + "\n",
      );

      runSync(sourceDir, enJsonPath);

      const out = JSON.parse(readFileSync(enJsonPath, "utf8"));
      expect(out["pub.tc_desc.delegation01"]).toBe(
        "Two or more name servers improve resilience.",
      );
      expect(out["pub.tag.delegation.NOT_ENOUGH_NS_DEL.header"]).toBe(
        "Not enough nameservers",
      );
      expect(out["pub.tag.delegation.NOT_ENOUGH_NS_DEL.desc"]).toBe(
        "Too few servers makes the domain fragile.",
      );
      expect(out["pub.app_title"]).toBe("x");
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  it("extracts glossary definition and match keys", () => {
    const { dir, sourceDir, enJsonPath } = setupFixture();
    try {
      writeFileSync(
        resolve(sourceDir, "glossary.md"),
        [
          "# Public UI glossary",
          "",
          "## Glossary zone-apex",
          "",
          "Match: zone apex, apex",
          "",
          "Description:",
          "",
          "The zone apex is the top of your domain.",
          "",
          "## Glossary dnssec",
          "",
          "Match: DNSSEC",
          "",
          "Description:",
          "",
          "DNSSEC adds signatures to DNS answers.",
          "",
        ].join("\n"),
      );
      writeFileSync(enJsonPath, "{}\n");

      runSync(sourceDir, enJsonPath);

      const out = JSON.parse(readFileSync(enJsonPath, "utf8"));
      // Slugs keep hyphens; the .match line is captured verbatim.
      expect(out["pub.glossary.zone-apex"]).toBe(
        "The zone apex is the top of your domain.",
      );
      expect(out["pub.glossary.zone-apex.match"]).toBe("zone apex, apex");
      expect(out["pub.glossary.dnssec"]).toBe(
        "DNSSEC adds signatures to DNS answers.",
      );
      expect(out["pub.glossary.dnssec.match"]).toBe("DNSSEC");
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  it("lowercases the module name in tag keys", () => {
    const { dir, sourceDir, enJsonPath } = setupFixture();
    try {
      writeFileSync(
        resolve(sourceDir, "DNSSEC.md"),
        [
          "## Tag BROKEN_CHAIN",
          "",
          "Header: Broken chain",
          "",
          "Description:",
          "",
          "The DNSSEC chain does not validate.",
          "",
        ].join("\n"),
      );
      writeFileSync(enJsonPath, "{}\n");

      runSync(sourceDir, enJsonPath);

      const out = JSON.parse(readFileSync(enJsonPath, "utf8"));
      expect(out["pub.tag.dnssec.BROKEN_CHAIN.header"]).toBe("Broken chain");
      expect(out["pub.tag.dnssec.BROKEN_CHAIN.desc"]).toBe(
        "The DNSSEC chain does not validate.",
      );
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  it("--check exits 0 when the catalog is up to date", () => {
    const { dir, sourceDir, enJsonPath } = setupFixture();
    try {
      writeFileSync(
        resolve(sourceDir, "delegation.md"),
        [
          "## Testcase delegation01",
          "",
          "Description:",
          "",
          "Two or more name servers.",
          "",
        ].join("\n"),
      );
      writeFileSync(enJsonPath, "{}\n");
      runSync(sourceDir, enJsonPath);
      expect(() => runSync(sourceDir, enJsonPath, ["--check"])).not.toThrow();
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });

  it("--check exits non-zero when the catalog is stale", () => {
    const { dir, sourceDir, enJsonPath } = setupFixture();
    try {
      writeFileSync(
        resolve(sourceDir, "delegation.md"),
        [
          "## Testcase delegation01",
          "",
          "Description:",
          "",
          "Fresh content.",
          "",
        ].join("\n"),
      );
      writeFileSync(enJsonPath, "{}\n");

      let error;
      try {
        runSync(sourceDir, enJsonPath, ["--check"]);
      } catch (e) {
        error = e;
      }
      expect(error).toBeDefined();
      expect(error.status).toBe(1);
      // The diagnostic must name the catalog that was actually checked, so a
      // caller pointing the script at some other file is not told to go and
      // fix the default one.
      expect(error.stderr).toContain(enJsonPath);
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
