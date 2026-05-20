# Installing gonemaster from packages

Linux packages (`.deb` and `.rpm`) are produced for `linux/amd64` and
`linux/arm64`. They install the binaries, man pages, systemd unit, env file,
and a separate data package for the badkeys blocklist.

## Packages

| Package | Architecture | Contents |
|---|---|---|
| `gonemaster` | amd64, arm64 | CLI binary at `/usr/bin/gonemaster`. Depends on `gonemaster-badkeys-data`. |
| `gonemaster-server` | amd64, arm64 | HTTP server with the embedded admin and public UIs. Ships the systemd unit and `/etc/gonemaster/server.env`. Depends on `gonemaster-badkeys-data`. Conflicts with `gonemaster-server-nogui`. |
| `gonemaster-server-nogui` | amd64, arm64 | API-only server, smaller binary, no UI. Same systemd unit and env file. Conflicts with `gonemaster-server`. |
| `gonemaster-client` | amd64, arm64 | CLI client for the server REST API at `/usr/bin/gonemaster-client`. |
| `gonemaster-nagios` | amd64, arm64 | Nagios/Icinga plugin at `/usr/lib/nagios/plugins/check_gonemaster`. |
| `gonemaster-badkeys-data` | all/noarch | Compromised-key blocklist data refreshed independently of the binaries. |

The server packages are mutually exclusive: installing one replaces the other
cleanly via `apt install` or `dnf install`.

## Where to get packages

For now, packages are attached to the [Codeberg release](https://codeberg.org/pawal/gonemaster/releases)
for each tagged version. A `SHA256SUMS` file is published alongside; verify
downloads before installing.

A Forgejo package registry hosted on Codeberg is on the roadmap; once enabled
it will support `apt install` / `dnf install` against a registry URL without
manual download. The implementation tasks are tracked in
[plans/packaging.md](../../plans/packaging.md).

## Install

### Debian / Ubuntu

    # Download the .deb files for your architecture from the release page.
    sudo apt install \
        ./gonemaster-badkeys-data_1.4.9_all.deb \
        ./gonemaster-server_1.4.9_amd64.deb

`apt` resolves the `Depends:` relationship and installs the data package
automatically. Add `./gonemaster_1.4.9_amd64.deb` and others as needed.

### Fedora / RHEL / Rocky

    sudo dnf install \
        ./gonemaster-badkeys-data-1.4.9-1.noarch.rpm \
        ./gonemaster-server-1.4.9-1.x86_64.rpm

### Verifying the install

    systemctl status gonemaster-server      # should be inactive after install
    man gonemaster-server                   # man page is present
    /usr/bin/gonemaster-server --version

## First start

The server is **not** started automatically. Review the env file, then enable
and start the service.

    sudo $EDITOR /etc/gonemaster/server.env
    sudo systemctl enable --now gonemaster-server

The default config:

- listens on `127.0.0.1:8080` (loopback only)
- uses SQLite at `/var/lib/gonemaster/gonemaster.db`
- runs as the `gonemaster` system user
- writes only to `/var/lib/gonemaster/` (everything else is read-only via systemd hardening)

To check it is alive:

    curl -s http://127.0.0.1:8080/api/v1/health
    journalctl -u gonemaster-server -f

## Exposing the server

The default loopback bind is deliberate. Put nginx, caddy, or apache in front
to terminate TLS and forward to `127.0.0.1:8080`. When you do, also set:

    GONEMASTER_TRUSTED_PROXY_CIDRS=127.0.0.1/32,::1/128

in `/etc/gonemaster/server.env` so the server reads client IPs from
`X-Forwarded-For`. See [public-api-and-proxy.md](public-api-and-proxy.md) for
the reverse-proxy hardening checklist.

## Choosing UI vs nogui

| Want | Install |
|---|---|
| Browse jobs and results in a web UI | `gonemaster-server` |
| API-only deployment behind a different frontend | `gonemaster-server-nogui` |

To switch, install the other package; apt/dnf removes the previous one. The
env file, systemd unit, and `/var/lib/gonemaster/` are preserved across the
swap.

## Common operations

| Action | Command |
|---|---|
| Reload after editing the env file | `sudo systemctl restart gonemaster-server` |
| View logs | `journalctl -u gonemaster-server` |
| Stop temporarily | `sudo systemctl stop gonemaster-server` |
| Disable on boot | `sudo systemctl disable gonemaster-server` |

## Updating

Drop in the newer `.deb` or `.rpm` and reinstall. Package upgrades:

- replace the binary and man pages,
- leave `/etc/gonemaster/server.env` untouched (marked config noreplace),
- preserve `/var/lib/gonemaster/` and its contents,
- restart the service only if it was running.

To pull a fresh badkeys blocklist without rebuilding the server, upgrade only
`gonemaster-badkeys-data`:

    sudo apt install ./gonemaster-badkeys-data_NEWVERSION_all.deb
    # or
    sudo dnf install ./gonemaster-badkeys-data-NEWVERSION-1.noarch.rpm

## Uninstall

    # Debian/Ubuntu
    sudo apt remove gonemaster-server gonemaster-badkeys-data
    sudo apt purge gonemaster-server     # additionally wipes /etc/gonemaster

    # Fedora/RHEL
    sudo dnf remove gonemaster-server gonemaster-badkeys-data

`remove` leaves the `gonemaster` system user and `/var/lib/gonemaster/` in
place so an accidental remove-and-reinstall keeps the database. To wipe
everything:

    sudo userdel gonemaster
    sudo groupdel gonemaster
    sudo rm -rf /var/lib/gonemaster /etc/gonemaster
