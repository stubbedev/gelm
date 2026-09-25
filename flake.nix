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
      in {
        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            # Go toolchain. gopls and golangci-lint are built with the same
            # nixpkgs Go, so they parse what the compiler accepts.
            go

            # Development tools
            gopls # Go language server
            golangci-lint # Linter behind `just lint`, config in .golangci.yml
            delve # Go debugger
            just # Task runner
          ];

          shellHook = ''
            # gelm is pure Go by design; keep accidental cgo out.
            export CGO_ENABLED=0
          '';
        };
      });
}
