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
      hash = "sha256-+h+IQN68OU4lMzpb4So0LJjfMEJmLvpgeHNetV8y9L0=";
    };
    public = {
      dir = "ui-public";
      dist = "../server/public/dist";
      hash = "sha256-P9C8iYeB7mieHkxnZrC8TgDk9MdJ2cwEXxtveK44llc=";
    };
    analysis = {
      dir = "analysis-ui";
      dist = "../server/analysisui/dist";
      hash = "sha256-WNEeZ+f1Qbh1BA/T/UdWkWMJWKWXciqhVUqyOGRvPb8=";
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
