# Gonemaster UI

This is a Svelte + Vite frontend that gets embedded into the `gonemaster-server` binary.

## Develop locally

```bash
cd ui
npm install
npm run dev
```

The dev server expects the API at the same origin. To point at a different API host, use the "API Connection" input inside the UI.

## Build for embedding

```bash
cd ui
npm install
npm run build
```

Build output lands in `server/ui/dist`. The Go server embeds everything from that directory when you compile `gonemaster-server`.
