{
  description = "gelm - pure-Go Wayland widget toolkit";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    devenv.url = "github:cachix/devenv";
  };

  outputs = { self, nixpkgs, flake-utils, devenv, ... } @ inputs:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        inherit (pkgs) lib;

        # One Go toolchain for every package build. go.mod declares
        # `go 1.27.1` and buildGoModule compiles with GOTOOLCHAIN=local,
        # so the version is load-bearing: nixpkgs' default `go` (1.26)
        # cannot compile this module, so the pin is go_1_27, shared with
        # devenv.nix's dev-shell pin - no toolchain is ever downloaded
        # at build or dev time.
        go = pkgs.go_1_27;
        buildGoModule = pkgs.buildGoModule.override { inherit go; };

        # Shared builder for every gelm derivation: pure Go (CGO off),
        # modules fetched through buildGoModule's standard flow with
        # vendorHash pinning go.sum, so no build reaches the network.
        # The test suite lives in CI (`just check` + `just headless`),
        # not in the build sandbox.
        mkGelm = { pname, subPackages ? [ ], description ? "", mainProgram ? null }:
          buildGoModule {
            inherit pname subPackages;
            version = self.shortRev or "dev";
            src = self;
            env.CGO_ENABLED = "0";
            vendorHash = "sha256-iZuwAslRVoSKAo/cZBVjjPHqcENi0tPj5JEaw/WLrC4=";
            doCheck = false;

            meta = with lib; {
              license = licenses.mit;
              platforms = platforms.linux;
              inherit description;
            } // lib.optionalAttrs (mainProgram != null) { inherit mainProgram; };
          };
      in
        # The module itself: one build of every package in the repo -
        # all demo binaries plus the wlpointer test driver. Downstream
        # nix Go builds can also reach gelm.goModules for the vendored
        # dependency tree; plain Go consumers pin this repo by semver
        # tag in go.mod (see README, Packaging).
        let
          gelm = mkGelm {
            pname = "gelm";
            description = "pure-Go Wayland widget toolkit (module and demos)";
          };
        in {
        packages = rec {
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

        # The whole dev environment lives in devenv.nix - one
        # definition for the devenv CLI and this wrapper alike, so
        # CI's `devenv shell -- just ...` and a flake user's
        # `nix develop --no-pure-eval` evaluate the same toolchains and
        # gates. The flag lets devenv read the working directory as the
        # project root, which pure flake evaluation cannot see.
        devShells.default = devenv.lib.mkShell {
          inherit inputs pkgs;
          modules = [ ./devenv.nix ];
        };

        checks = {
          # The Hyprland side of the compositor-in-the-loop gate (#66):
          # a private NixOS VM (own kernel, own virtio-gpu DRM node,
          # own seatd) that boots Hyprland and runs the same suite the
          # sway gate runs. Needs KVM; see tests/hyprland-vm.nix.
          gelm-hyprland-vm = pkgs.callPackage ./tests/hyprland-vm.nix {
            src = self;
            goModules = gelm.goModules;
          };
        };
      });
}
