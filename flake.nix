# This flake works by simply using go get <package> to fetch dependencies.
# It uses vendor/ for dependencies, so you can run `go mod vendor` to populate it.
# Or run `task mod` to automatically fetch dependencies and populate the vendor directory.

# To build the Go project, run:
#   task build

# To run the Go project, run:
#   nix run

{
  description = "NOS - Protogen :3";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs?ref=nixos-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };

        pythonEnv = pkgs.python3.withPackages (ps: with ps; [
          pyyaml
          pillow
          numpy
          rich
        ]);

      in {
        packages = {
          default = pkgs.buildGoModule {
            pname = "put-name-here";  # Set the name of your package
            version = "0.0.1";

            src = pkgs.lib.cleanSource ./.;

            env.CGO_ENABLED = 1;

            ldflags = [
              "-s" "-w" "-extldflags '-static'"
            ];  # Strip Binary and Disable Debug Information, static linking

            vendorHash = null;  # Null if you don't have a vendor directory

            buildInputs = [
              pkgs.musl
              pkgs.go
              pkgs.gopls
              pkgs.gotools
              pkgs.go-tools
              pkgs.golangci-lint
              pkgs.tinygo
              pkgs.picotool
            ];
          };

          gopls = pkgs.gopls;
        };

        devShells.default = pkgs.mkShell {
          nativeBuildInputs = [
            # Go tooling
            pkgs.go
            pkgs.go-task
            pkgs.gopls
            pkgs.golangci-lint
            pkgs.gotools
            pkgs.go-tools
            pkgs.gotestsum  # prettier `task test:pretty` output
            pkgs.musl
            pkgs.tinygo
            pkgs.picotool

            # Python/Micropython tooling (from compiler)
            pkgs.python3
            pkgs.micropython
            pkgs.rshell
            pkgs.mpy-utils
            pkgs.adafruit-ampy
            pkgs.picocom
          ];

          buildInputs = [
            pythonEnv
            pkgs.libclang
          ];

          shellHook = ''
            if [ "$SHELL" = "$(which fish)" ]; then
              source .dev-fish-setup.fish
            fi
          '';
        };
      }
    );
}
