{
  description = "ot - Obsidian Tasks";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            gotools
            go-tools
            just
            vhs
          ];

          shellHook = ''
            export GOPATH="$HOME/go"
            export PATH="$GOPATH/bin:$PATH"
            echo "ot development environment loaded"
            echo "Go version: $(go version)"
          '';
        };

        packages.default = pkgs.buildGoModule {
          pname = "ot";
          version = "0.1.0";
          src = ./.;
          vendorHash = null; # Will need to be updated after go.sum exists
        };
      }
    );
}
