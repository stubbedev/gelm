{
  perSystem = { buildGoModule, go, lib, pkgs, ... }:
    let
      # Rebuild a nixpkgs Go tool against the pinned toolchain: every
      # buildGo*Module argument its override takes (gopls asks for
      # buildGoLatestModule, the rest for buildGoModule) gets ours, so
      # gopls, delve and the editor tools understand go 1.27 sources.
      withPinnedGo = pkg:
        pkg.override (lib.mapAttrs (_: _: buildGoModule)
          (lib.filterAttrs (name: _: lib.match "buildGo.*Module" name != null)
            (lib.functionArgs pkg.override)));
    in
    {
      devShells.default = pkgs.mkShellNoCC {
        packages = [
          go
          pkgs.git

          # Development tools
          pkgs.golangci-lint # `just lint`, and `just fmt` through its gofumpt + gci formatters; config in .golangci.yml
          pkgs.just # task runner
        ]
        # Headless test compositor: sway on WLR_BACKENDS=headless gives
        # input integration tests a private, deterministic Wayland session
        # (layer shell + virtual pointer protocol included) without
        # touching the developer's real desktop. Linux only.
        ++ lib.optionals pkgs.stdenv.hostPlatform.isLinux [ pkgs.sway ]
        ++ [
          # `just atspi`'s bus-level tests launch a private dbus-daemon as
          # the accessibility bus and skip when it is missing; shipping it
          # keeps that gate honest wherever the dev shell runs.
          pkgs.dbus
          # `just svg-refs` rasterizes the SVG references render.LoadSVG
          # is compared against with rsvg-convert.
          pkgs.librsvg
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

        # lys-ui is pure Go by design; keep accidental cgo out. The
        # package builds set the same thing.
        CGO_ENABLED = "0";

        # nixpkgs' go defaults to GOTOOLCHAIN=auto, which would fetch
        # whatever toolchain a go.mod line asks for; the shell holds
        # to the pinned one, as the package builds do.
        GOTOOLCHAIN = "local";

        # `just atspi-verify` runs the bridge against real at-spi2-core: its
        # bus launcher and registry daemon live in libexec, off PATH.
        ATSPI_LIBEXEC = "${pkgs.at-spi2-core}/libexec";

        # `just headless` points the suite's clients at a known xcursor theme:
        # the client-side cursor path (LYS_UI_NO_CURSOR_SHAPE) needs one, and a
        # CI runner has none. Only the recipe exports it - a developer's own
        # cursor setup inside the shell is untouched.
        LYS_UI_TEST_XCURSOR_PATH = "${pkgs.vanilla-dmz}/share/icons";
      };
    };
}
