{
  description = "transmission-protonvpn-nat-pmp";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    gomod2nix = {
      url = "github:tweag/gomod2nix";
      inputs = {
        nixpkgs.follows = "nixpkgs";
        flake-utils.follows = "flake-utils";
      };
    };
  };

  outputs = {
    self,
    nixpkgs,
    flake-utils,
    gomod2nix,
    ...
  }:
    flake-utils.lib.eachDefaultSystem (system: let
      pkgs = import nixpkgs {
        inherit system;
        overlays = [(import "${gomod2nix}/overlay.nix")];
      };
    in {
      packages.default = pkgs.buildGoApplication {
        name = "transmission-protonvpm-nat-pmp";
        pwd = ./.;
        src = ./.;
      };
      devShells.default = pkgs.mkShell {
        inputsFrom = [
          self.packages.${system}.default
        ];

        buildInputs = [
          pkgs.gomod2nix
          pkgs.revive
          pkgs.go-tools
        ];
      };
    });
}
