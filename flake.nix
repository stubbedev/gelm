{
  description = "gelm - pure-Go Wayland widget toolkit";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        inherit (pkgs) lib;

        # One Go toolchain for the dev shell and every package build.
        # go.mod declares `go 1.27.1` and buildGoModule compiles with
        # GOTOOLCHAIN=local, so the version is load-bearing: nixpkgs'
        # default `go` (1.26) cannot compile this module, so the pin is
        # go_1_27, shared by the dev shell and buildGoModule alike -
        # no toolchain is ever downloaded at build or dev time.
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
            vendorHash = "sha256-y5igtdYZ61+SWmsP/8Ikq/SoBMi/FJTHfz/BVvZbENE=";
            doCheck = false;

            meta = with lib; {
              license = licenses.mit;
              platforms = platforms.linux;
              inherit description;
            } // lib.optionalAttrs (mainProgram != null) { inherit mainProgram; };
          };
      in {
        packages = rec {
          # The module itself: one build of every package in the repo -
          # all demo binaries plus the wlpointer test driver. Downstream
          # nix Go builds can also reach gelm.goModules for the vendored
          # dependency tree; plain Go consumers pin this repo by semver
          # tag in go.mod (see README, Packaging).
          gelm = mkGelm {
            pname = "gelm";
            description = "pure-Go Wayland widget toolkit (module and demos)";
          };

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

        devShells.default = pkgs.mkShell {
          packages = [
            # The pinned Go toolchain (see `go` above); gopls and
            # golangci-lint are nixpkgs builds, close enough to this Go
            # that they parse what the compiler accepts.
            go

            # Development tools
            pkgs.gofumpt # stricter gofmt; `just fmt`, CI's formatting gate
            pkgs.gopls # Go language server
            pkgs.golangci-lint # Linter behind `just lint`, config in .golangci.yml
            pkgs.delve # Go debugger
            pkgs.just # Task runner

            # Headless test compositor: sway on WLR_BACKENDS=headless gives
            # input integration tests a private, deterministic Wayland
            # session (layer shell + virtual pointer protocol included)
            # without touching the developer's real desktop.
            pkgs.sway
          ];

          shellHook = ''
            # gelm is pure Go by design; keep accidental cgo out.
            export CGO_ENABLED=0
          '';
        };
      });
}
