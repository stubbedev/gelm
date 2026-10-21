{ self, ... }: {
  perSystem = { buildGoModule, lib, pkgs, ... }:
    let
      # Shared builder for every gelm derivation: pure Go (CGO off),
      # modules fetched through buildGoModule's standard flow with
      # vendorHash pinning go.sum, so no build reaches the network.
      # The test suite lives in CI (`just check` + `just headless`),
      # not in the build sandbox.
      mkGelm = { pname, subPackages ? [ ], description ? "", mainProgram ? null }:
        buildGoModule {
          inherit pname subPackages;
          version = self.shortRev or "dev";
          src = lib.fileset.toSource {
            root = ../.;
            fileset = lib.fileset.difference (lib.fileset.gitTracked ../.) (
              lib.fileset.unions [
                ../flake.nix
                ../flake.lock
                ../nix
              ]
            );
          };
          env.CGO_ENABLED = "0";
          vendorHash = "sha256-NH6lBAUZwa7nuoC7ftPnE9zQn4VH9xGDKf7ePS8px3c=";
          doCheck = false;
          # third_party/ holds patched upstream packages gelm imports;
          # they build as dependencies, not as targets of their own.
          excludedPackages = [ "third_party" ];

          meta = with lib; {
            license = licenses.mit;
            platforms = platforms.linux;
            inherit description;
          } // lib.optionalAttrs (mainProgram != null) { inherit mainProgram; };
        };

      # The module itself: one build of every package in the repo -
      # all demo binaries plus the wlpointer test driver. Downstream
      # nix Go builds can also reach gelm.goModules for the vendored
      # dependency tree; plain Go consumers pin this repo by semver
      # tag in go.mod (see README, Packaging).
      gelm = mkGelm {
        pname = "gelm";
        description = "pure-Go Wayland widget toolkit (module and demos)";
      };
    in
    {
      # A Wayland toolkit: the builds exist on Linux only, while the dev
      # shell stays available on every system.
      packages = lib.optionalAttrs pkgs.stdenv.hostPlatform.isLinux {
        inherit gelm;

        gelm-bar = mkGelm {
          pname = "gelm-bar";
          subPackages = [ "cmd/gelm-bar" ];
          description = "gelm layer-shell status bar";
          mainProgram = "gelm-bar";
        };

        gelm-panel = mkGelm {
          pname = "gelm-panel";
          subPackages = [ "cmd/gelm-panel" ];
          description = "gelm layer-shell panel";
          mainProgram = "gelm-panel";
        };

        gelm-hello = mkGelm {
          pname = "gelm-hello";
          subPackages = [ "cmd/gelm-hello" ];
          description = "gelm widget showcase";
          mainProgram = "gelm-hello";
        };

        default = gelm;
      };
    };
}
