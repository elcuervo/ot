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
        version = builtins.replaceStrings ["\n"] [""] (builtins.readFile ./VERSION);
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
            export PATH="$PWD:$PATH"
            echo "ot development environment loaded"
            echo "Go version: $(go version)"
          '';
        };

        packages = {
          ot = pkgs.buildGoModule {
            pname = "ot";
            inherit version;
            src = ./.;
            vendorHash = "sha256-N51t/7t0I9MaaZUPswoAJf5sah7TIC7gT6HRrvQXYYI=";
          };
          default = self.packages.${system}.ot;
        };
      }
    );
}
