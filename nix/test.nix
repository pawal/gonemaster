# NixOS VM test for the gonemaster-server module.
{ pkgs, ... }:

let
  # Test-only admin token and its hash, from gonemaster-server auth add-token.
  token = "gm_xuJ7Si9Yr-C6ReVNhRxw52g1iAXjZXY8ktFHNtKfztY";
  tokenHash = "vmtest=sha256:7d92688741b660242238bc204bb5914488bc625d27890b6f301e6af6221ac04b";
in
{
  name = "gonemaster-server";

  nodes.machine = {
    services.gonemaster-server = {
      enable = true;
      # The VM has no route out, so every query times out.
      settings = {
        timeout = 1;
        retry = 1;
        database.retention_days = 30;
      };
      environmentFile = pkgs.writeText "gonemaster-server.env" ''
        GONEMASTER_ADMIN_TOKEN_HASHES=${tokenHash}
      '';
    };
  };

  testScript = ''
    import json

    base = "http://127.0.0.1:8080"
    auth = "Authorization: Bearer ${token}"
    client = f"gonemaster-client --no-color --server {base}/api/v1 --header '{auth}'"

    machine.wait_for_unit("gonemaster-server.service")
    machine.wait_for_open_port(8080)

    with subtest("settings reach the server"):
        path = machine.succeed("systemctl cat gonemaster-server | grep -o '/nix/store/[^ ]*-gonemaster-server.json'").strip()
        cfg = json.loads(machine.succeed(f"gonemaster-server --config {path} --dump-config"))
        assert cfg["database"]["driver"] == "sqlite", cfg
        assert cfg["database"]["retention_days"] == 30, cfg
        assert cfg["timeout"] == 1, cfg

    with subtest("embedded web UIs are served"):
        machine.succeed(f"curl -sf {base}/ | grep -F '<title>Gonemaster</title>'")
        machine.succeed(f"curl -sf {base}/public/ | grep -F '<title>Gonemaster</title>'")
        machine.succeed(f"curl -sf {base}/analysis/ | grep -F '<title>Gonemaster Analysis</title>'")

    with subtest("environment file gates the admin API"):
        machine.succeed(f"test \"$(curl -s -o /dev/null -w '%{{http_code}}' {base}/api/v1/jobs)\" = 401")
        machine.succeed(f"curl -sf -H '{auth}' {base}/api/v1/jobs")

    with subtest("a job runs in the sandboxed service"):
        out = machine.succeed(f"{client} jobs create --domain example.com --wait --view summary")
        assert "(succeeded)" in out, out

    with subtest("state survives a restart"):
        machine.succeed("test -s /var/lib/gonemaster/gonemaster.db")
        machine.systemctl("restart gonemaster-server.service")
        machine.wait_for_open_port(8080)
        machine.succeed(f"{client} jobs list | grep -F example.com")
  '';
}
