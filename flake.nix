{
  description = "tasmota-exporter";

  inputs = {
    nixpkgs.url = "nixpkgs/nixpkgs-unstable";
    utils.url = "github:numtide/flake-utils";
  };

  outputs = {
    self,
    nixpkgs,
    utils,
    ...
  }: let
    tasmota-exporterVersion =
      if (self ? shortRev)
      then self.shortRev
      else "dev";
  in
    {
      overlay = _: prev: let
        pkgs = nixpkgs.legacyPackages.${prev.system};
      in {
        tasmota-exporter = pkgs.buildGoModule {
          pname = "tasmota-exporter";
          version = tasmota-exporterVersion;
          src = pkgs.nix-gitignore.gitignoreSource [] ./.;

          subPackage = ["cmd/tasmota-exporter"];

          vendorHash = "sha256-zN1AlvLzE2X/OUH6RzoCP+5tY46s9uWPeFfgTt3jNUw=";
        };
      };
    }
    // utils.lib.eachDefaultSystem
    (system: let
      pkgs = import nixpkgs {
        overlays = [self.overlay];
        inherit system;
      };
      buildDeps = with pkgs; [
        git
        gnumake
        go
      ];
      devDeps = with pkgs;
        buildDeps
        ++ [
          golangci-lint
          entr
          nodePackages.tailwindcss
        ];
    in rec {
      # `nix develop`
      devShell = pkgs.mkShell {
        buildInputs = devDeps;
      };

      # `nix build`
      packages = with pkgs; {
        inherit tasmota-exporter;
      };

      defaultPackage = pkgs.tasmota-exporter;

      # `nix run`
      apps = {
        tasmota-exporter = utils.lib.mkApp {
          drv = packages.tasmota-exporter;
        };
      };

      defaultApp = apps.tasmota-exporter;

      overlays.default = self.overlay;
    })
    // {
      nixosModules.default = {
        pkgs,
        lib,
        config,
        ...
      }: let
        cfg = config.services.tasmota-exporter;
      in {
        options = with lib; {
          services.tasmota-exporter = {
            enable = mkEnableOption "Enable tasmota-exporter";

            package = mkOption {
              type = types.package;
              description = ''
                tasmota-exporter package to use
              '';
              default = pkgs.tasmota-exporter;
            };

            listenAddr = mkOption {
              type = types.str;
              default = ":9090";
            };
          };
        };
        config = lib.mkIf cfg.enable {
          systemd.services.tasmota-exporter = {
            enable = true;
            script = ''
              export TASMOTA_EXPORTER_LISTEN_ADDR=${cfg.listenAddr}
              ${cfg.package}/bin/tasmota-exporter
            '';
            wantedBy = ["multi-user.target"];
            after = ["network-online.target"];
            serviceConfig = {
              DynamicUser = true;
              Restart = "always";
              RestartSec = "15";
            };
            path = [cfg.package];
          };
        };
      };
    };
}
