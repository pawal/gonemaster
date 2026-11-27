# NixOS module for gonemaster-server.
{
  config,
  lib,
  pkgs,
  ...
}:

let
  cfg = config.services.gonemaster-server;
  format = pkgs.formats.json { };
  configFile = format.generate "gonemaster-server.json" cfg.settings;
in
{
  options.services.gonemaster-server = {
    enable = lib.mkEnableOption "gonemaster-server, the gonemaster HTTP API and web UIs";

    package = lib.mkPackageOption pkgs "gonemaster" { };

    settings = lib.mkOption {
      type = lib.types.submodule {
        freeformType = format.type;
        options = {
          listen_addr = lib.mkOption {
            type = lib.types.str;
            default = "127.0.0.1:8080";
            description = "Address and port of the HTTP listener.";
          };
          database = lib.mkOption {
            type = lib.types.submodule {
              freeformType = format.type;
              options = {
                driver = lib.mkOption {
                  type = lib.types.enum [
                    "memory"
                    "sqlite"
                    "postgres"
                    "mariadb"
                  ];
                  default = "sqlite";
                  description = "Storage backend.";
                };
                dsn = lib.mkOption {
                  type = lib.types.str;
                  default = "/var/lib/gonemaster/gonemaster.db";
                  description = "SQLite file path, or a PostgreSQL or MariaDB connection string without a password.";
                };
              };
            };
            default = { };
            description = "Database settings.";
          };
        };
      };
      default = { };
      example = {
        public_url = "https://gonemaster.example/";
        database.retention_days = 90;
        public_api.rate_limit_enabled = true;
      };
      description = ''
        Server configuration, written to the JSON file passed with `--config`.
        The keys are those of the config file in docs/server/configuration.md.
        The file is world readable in the Nix store; put secrets in
        {option}`services.gonemaster-server.environmentFile`.
      '';
    };

    environmentFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      example = "/run/secrets/gonemaster-server.env";
      description = ''
        File with `GONEMASTER_*` variables, such as `GONEMASTER_DB_DSN` or
        `GONEMASTER_ADMIN_TOKEN_HASHES`. They override
        {option}`services.gonemaster-server.settings`.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    environment.systemPackages = [ cfg.package ];

    systemd.services.gonemaster-server = {
      description = "gonemaster DNS test server";
      documentation = [ "man:gonemaster-server(1)" ];
      wantedBy = [ "multi-user.target" ];
      wants = [ "network-online.target" ];
      after = [ "network-online.target" ];
      restartTriggers = [ configFile ];

      serviceConfig = {
        ExecStart = "${lib.getExe' cfg.package "gonemaster-server"} --config ${configFile}";
        ExecReload = "${pkgs.coreutils}/bin/kill -HUP $MAINPID";
        EnvironmentFile = lib.optional (cfg.environmentFile != null) cfg.environmentFile;
        Restart = "on-failure";
        RestartSec = 5;

        DynamicUser = true;
        StateDirectory = "gonemaster";
        StateDirectoryMode = "0750";
        WorkingDirectory = "/var/lib/gonemaster";
        UMask = "0077";

        CapabilityBoundingSet = "";
        NoNewPrivileges = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        PrivateTmp = true;
        PrivateDevices = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectKernelLogs = true;
        ProtectControlGroups = true;
        ProtectClock = true;
        ProtectHostname = true;
        ProtectProc = "invisible";
        RestrictAddressFamilies = [
          "AF_INET"
          "AF_INET6"
          "AF_UNIX"
          "AF_NETLINK"
        ];
        RestrictSUIDSGID = true;
        RestrictRealtime = true;
        RestrictNamespaces = true;
        LockPersonality = true;
        MemoryDenyWriteExecute = true;
        SystemCallArchitectures = "native";
        SystemCallFilter = [
          "@system-service"
          "~@privileged"
        ];
        SystemCallErrorNumber = "EPERM";
      };
    };
  };

  meta.maintainers = [ ];
}
