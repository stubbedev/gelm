{
  pkgs,
  ...
}:

{
  # gelm is pure Go by design; keep accidental cgo out. The package
  # builds in flake.nix set the same thing.
  env.CGO_ENABLED = "0";

  # The pinned Go toolchain: go.mod declares `go 1.27.1` and nixpkgs'
  # default `go` (1.26) cannot compile this module, so the version is
  # load-bearing. flake.nix pins the same go_1_27 for its
  # buildGoModule, and the language module rebuilds gopls and delve
  # against this exact toolchain instead of nixpkgs' own builds - no
  # toolchain is ever downloaded at build or dev time.
  languages.go.enable = true;
  languages.go.package = pkgs.go_1_27;

  packages = [
    pkgs.git

    # Development tools
    pkgs.gofumpt # stricter gofmt; `just fmt`, CI's formatting gate
    pkgs.golangci-lint # linter behind `just lint`, config in .golangci.yml
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
  ];

  # `just atspi-verify` runs the bridge against real at-spi2-core: its
  # bus launcher and registry daemon live in libexec, off PATH.
  env.ATSPI_LIBEXEC = "${pkgs.at-spi2-core}/libexec";

  # `devenv test` = the full CI gate (`just check-headless`): the
  # release gates, then the compositor-in-the-loop input suite where
  # GELM_HEADLESS=1 turns every skip into a failure.
  enterTest = ''
    just check-headless
  '';
}
