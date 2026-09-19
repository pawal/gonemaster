{
  description = "gonemaster, a DNS delegation, zone, and DNSSEC test engine";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];

      forAllSystems = fn: nixpkgs.lib.genAttrs systems (system: fn nixpkgs.legacyPackages.${system});

      # The UI derivations read their lockfiles under a fixed root name.
      src = builtins.path {
        name = "source";
        path = ./.;
      };
    in
    {
      overlays.default = final: _prev: {
        gonemaster = final.callPackage ./nix/package.nix { inherit src; };
      };

      packages = forAllSystems (pkgs: rec {
        gonemaster = pkgs.callPackage ./nix/package.nix { inherit src; };
        gonemaster-nogui = gonemaster.override { withUI = false; };
        default = gonemaster;
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = [
            pkgs.go_1_27
            pkgs.nodejs
            pkgs.gnumake
            pkgs.go-md2man
          ];
        };
      });
    };
}
