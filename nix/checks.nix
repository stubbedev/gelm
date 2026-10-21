{
  perSystem = { go, lib, pkgs, self', ... }: {
    checks = lib.optionalAttrs pkgs.stdenv.hostPlatform.isLinux {
      # The Hyprland side of the compositor-in-the-loop gate (#66):
      # a private NixOS VM (own kernel, own virtio-gpu DRM node,
      # own seatd) that boots Hyprland and runs the same suite the
      # sway gate runs. Needs KVM; see tests/hyprland-vm.nix.
      gelm-hyprland-vm = pkgs.callPackage ../tests/hyprland-vm.nix {
        inherit go;
        inherit (self'.packages.gelm) src goModules;
      };
    };
  };
}
