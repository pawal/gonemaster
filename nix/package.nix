{
  lib,
  buildGo127Module,
  buildNpmPackage,
  fetchFromGitea,
  go-md2man,
  installShellFiles,
  nix-update-script,
  nodejs,
  # A local tree, for the flake. Null fetches the release tag.
  src ? null,
  # False selects the nogui tag and skips the Node build entirely.
  withUI ? true,
}:

let
  version = "1.7.13";

  source =
    if src != null then
      src
    else
      fetchFromGitea {
        domain = "codeberg.org";
        owner = "pawal";
        repo = "gonemaster";
        tag = "v${version}";
        hash = lib.fakeHash;
      };

  ui = import ./ui.nix {
    inherit
      lib
      buildNpmPackage
      nodejs
      version
      ;
    src = source;
  };
in
buildGo127Module (finalAttrs: {
  pname = "gonemaster";
  inherit version;
  src = source;

  vendorHash = lib.fakeHash;

  subPackages = [
    "cmd/gonemaster"
    "cmd/gonemaster-server"
    "cmd/gonemaster-client"
    "cmd/gonemaster-nagios"
    "cmd/gonemaster-mcp"
  ];

  tags = lib.optionals (!withUI) [ "nogui" ];

  ldflags = [
    "-s"
    "-w"
    "-X main.version=${finalAttrs.version}"
  ];

  # The sqlite driver is pure Go, so the binaries stay static.
  env.CGO_ENABLED = 0;

  # Replace the committed placeholders with the built assets.
  preBuild = lib.optionalString withUI ''
    rm -rf server/ui/dist server/public/dist server/analysisui/dist
    cp -r ${ui.admin} server/ui/dist
    cp -r ${ui.public} server/public/dist
    cp -r ${ui.analysis} server/analysisui/dist
    chmod -R u+w server/ui/dist server/public/dist server/analysisui/dist
  '';

  nativeBuildInputs = [
    go-md2man
    installShellFiles
  ];

  # The man pages are generated from markdown and are not in the tree.
  postInstall = ''
    mkdir -p man
    for page in docs/man/*.1.md; do
      go-md2man -in "$page" -out "man/$(basename "$page" .md)"
    done
    installManPage man/*.1
  '';

  passthru.updateScript = nix-update-script { };

  meta = {
    description = "DNS delegation, zone, and DNSSEC test engine";
    homepage = "https://codeberg.org/pawal/gonemaster";
    changelog = "https://codeberg.org/pawal/gonemaster/src/tag/v${finalAttrs.version}/Changelog";
    license = lib.licenses.bsd2;
    mainProgram = "gonemaster";
    maintainers = [ ];
    platforms = lib.platforms.unix;
  };
})
