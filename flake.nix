{
  description = "tasmota-exporter";

  inputs = {
    nixpkgs.url = "nixpkgs/nixpkgs-unstable";
    utils.url = "github:numtide/flake-utils";
    flake-checks.url = "github:kradalby/flake-checks";
    flake-checks.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    { self
    , nixpkgs
    , utils
    , flake-checks
    , ...
    }:
    let
      tasmota-exporterVersion =
        if (self ? shortRev)
        then self.shortRev
        else "dev";
    in
    {
      overlays.default = _: prev:
        let
          pkgs = nixpkgs.legacyPackages.${prev.stdenv.hostPlatform.system};
        in
        {
          tasmota-exporter = pkgs.callPackage
            ({ buildGo126Module }:
              buildGo126Module {
                pname = "tasmota-exporter";
                version = tasmota-exporterVersion;
                src = pkgs.nix-gitignore.gitignoreSource [ ] ./.;

                subPackages = [ "cmd/tasmota-exporter" ];

                vendorHash = "sha256-bZqg+XnuEtWfja7C2OD9wZuNwYRkJ8b9kxyTdghcMlE=";
              })
            { };
        };
    }
    // utils.lib.eachDefaultSystem
      (system:
      let
        pkgs = import nixpkgs {
          overlays = [ self.overlays.default ];
          inherit system;
        };
        fc = flake-checks.lib;
        common = {
          inherit pkgs;
          root = ./.;
          pname = "tasmota-exporter";
          version = tasmota-exporterVersion;
          vendorHash = "sha256-bZqg+XnuEtWfja7C2OD9wZuNwYRkJ8b9kxyTdghcMlE=";
          goPkg = pkgs.go_1_26;
        };
      in
      {
        # `nix develop`
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            git
            gnumake
            go_1_26
            golangci-lint
            entr
          ];
        };

        formatter = fc.formatter common;

        # `nix flake check`
        checks = {
          build = fc.goBuild common;
          gotest = fc.goTest common;
          golangci-lint = fc.goLint common;
          formatting = fc.goFormat common;
        };

        # `nix build`
        packages = with pkgs; {
          inherit tasmota-exporter;
          default = tasmota-exporter;
        };

        # `nix run`
        apps = {
          tasmota-exporter = utils.lib.mkApp {
            drv = pkgs.tasmota-exporter;
          };
          default = utils.lib.mkApp {
            drv = pkgs.tasmota-exporter;
          };
        };
      })
    // {
      nixosModules.default =
        { pkgs
        , lib
        , config
        , ...
        }:
        let
          cfg = config.services.tasmota-exporter;
        in
        {
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
              wantedBy = [ "multi-user.target" ];
              after = [ "network-online.target" ];
              wants = [ "network-online.target" ];
              serviceConfig = {
                DynamicUser = true;
                Restart = "always";
                RestartSec = "15";
              };
              path = [ cfg.package ];
            };
          };
        };
    };
}
