import { mkdirSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";

const distDir = resolve(process.cwd(), "../server/analysisui/dist");
mkdirSync(distDir, { recursive: true });
writeFileSync(
  resolve(distDir, "placeholder.txt"),
  "This placeholder keeps the dist directory embeddable when built UI assets are not present.\n" +
    "Run `make ui-build` before building the default server binary to embed the analysis dashboard.\n"
);
