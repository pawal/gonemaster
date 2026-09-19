# The three Svelte apps, each built into the directory the server embeds.
{
  lib,
  buildNpmPackage,
  nodejs,
  src,
  version,
}:

let
  # Source directory, dist path relative to it, and the lockfile hash.
  apps = {
    admin = {
      dir = "ui";
      dist = "../server/ui/dist";
      hash = lib.fakeHash;
    };
    public = {
      dir = "ui-public";
      dist = "../server/public/dist";
      hash = lib.fakeHash;
    };
    analysis = {
      dir = "analysis-ui";
      dist = "../server/analysisui/dist";
      hash = lib.fakeHash;
    };
  };

  build =
    name: app:
    buildNpmPackage {
      pname = "gonemaster-${name}-ui";
      inherit version src nodejs;

      # Vite writes outside the app directory, so the whole tree is unpacked.
      sourceRoot = "source/${app.dir}";

      # unpackPhase makes only sourceRoot writable, and vite writes past it.
      postUnpack = "chmod -R u+w source";

      npmDepsHash = app.hash;

      installPhase = ''
        runHook preInstall
        cp -r ${app.dist} $out
        runHook postInstall
      '';

      meta = {
        description = "gonemaster ${name} web UI";
        license = lib.licenses.bsd2;
      };
    };
in
lib.mapAttrs build apps
