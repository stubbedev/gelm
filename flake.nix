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

        # One Go toolchain for every package build and the dev shell.
        # go.mod declares `go 1.27.1` and buildGoModule compiles with
        # GOTOOLCHAIN=local, so the version is load-bearing: nixpkgs'
        # default `go` (1.26) cannot compile this module, so the pin is
        # go_1_27 - no toolchain is ever downloaded at build or dev time.
        go = pkgs.go_1_27;
        buildGoModule = pkgs.buildGoModule.override { inherit go; };

        # Rebuild a nixpkgs Go tool against the pinned toolchain: every
        # buildGo*Module argument its override takes (gopls asks for
        # buildGoLatestModule, the rest for buildGoModule) gets ours, so
        # gopls, delve and the editor tools understand go 1.27 sources.
        withPinnedGo = pkg:
          pkg.override (lib.mapAttrs (_: _: buildGoModule)
            (lib.filterAttrs (name: _: lib.match "buildGo.*Module" name != null)
              (lib.functionArgs pkg.override)));

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

        devShells.default = pkgs.mkShellNoCC {
          packages = [
            go
            pkgs.git

            # Development tools
            pkgs.golangci-lint # `just lint`, and `just fmt` through its gofumpt + gci formatters; config in .golangci.yml
            pkgs.just # task runner

            # Headless test compositor: sway on WLR_BACKENDS=headless gives
            # input integration tests a private, deterministic Wayland session
            # (layer shell + virtual pointer protocol included) without
            # touching the developer's real desktop.
            pkgs.sway

            # `just atspi`'s bus-level tests launch a private dbus-daemon as
            # the accessibility bus and skip when it is missing; shipping it
            # keeps that gate honest wherever the dev shell runs.
            pkgs.dbus
          ]
          # Editor tooling (gopls, delve, and the tools vscode-go and
          # vim-go install), built against the pinned Go: vscode-go
          # expects every tool compiled with the Go it drives.
          ++ map withPinnedGo [
            pkgs.gopls
            pkgs.delve
            pkgs.gotools
            pkgs.gomodifytags
            pkgs.impl
            pkgs.go-tools
            pkgs.gotests
            pkgs.iferr
          ];

          # gelm is pure Go by design; keep accidental cgo out. The
          # package builds above set the same thing.
          CGO_ENABLED = "0";

          # nixpkgs' go defaults to GOTOOLCHAIN=auto, which would fetch
          # whatever toolchain a go.mod line asks for; the shell holds
          # to the pinned one, as the package builds do.
          GOTOOLCHAIN = "local";

          # `just atspi-verify` runs the bridge against real at-spi2-core: its
          # bus launcher and registry daemon live in libexec, off PATH.
          ATSPI_LIBEXEC = "${pkgs.at-spi2-core}/libexec";

          # `just headless` points the suite's clients at a known xcursor theme:
          # the client-side cursor path (GELM_NO_CURSOR_SHAPE) needs one, and a
          # CI runner has none. Only the recipe exports it - a developer's own
          # cursor setup inside the shell is untouched.
          GELM_TEST_XCURSOR_PATH = "${pkgs.vanilla-dmz}/share/icons";
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
