# Installing gonemaster from packages

Tagged releases ship Linux binary archives and Linux packages (`.deb` and
`.rpm`) for `linux/amd64` and `linux/arm64`. The packages install the
binaries, man pages, systemd unit, and env file.

## Packages

| Package | Architecture | Contents |
|---|---|---|
| `gonemaster` | amd64, arm64 | CLI binary at `/usr/bin/gonemaster`. Recommends `gonemaster-badkeys-data`. |
| `gonemaster-server` | amd64, arm64 | HTTP server with the embedded admin, public, and analysis UIs. Ships the systemd unit and `/etc/gonemaster/server.env`. Recommends `gonemaster-badkeys-data`. Conflicts with `gonemaster-server-nogui`. |
| `gonemaster-client` | amd64, arm64 | CLI client for the server REST API at `/usr/bin/gonemaster-client`. |
| `gonemaster-nagios` | amd64, arm64 | Nagios/Icinga plugin at `/usr/lib/nagios/plugins/check_gonemaster`. |
| `gonemaster-mcp` | amd64, arm64 | Model Context Protocol bridge at `/usr/bin/gonemaster-mcp`. |
| `gonemaster-server-nogui` | amd64, arm64 | API-only server, smaller binary, no UI. Same systemd unit and env file. Conflicts with `gonemaster-server`. Not attached to releases; build it with `make packages`. |
| `gonemaster-badkeys-data` | all/noarch | Compromised-key blocklist data refreshed independently of the binaries. Not attached to releases; see [Blocklist data](#blocklist-data). |

The server packages are mutually exclusive: installing one replaces the other
cleanly via `apt install` or `dnf install`.

## Release artifacts

Each tagged version attaches 23 files to its
[Codeberg release](https://codeberg.org/pawal/gonemaster/releases): two binary
archives, twenty packages, and `checksums.txt`.

An archive is named `gonemaster_vX.Y.Z_linux_<arch>.tar.gz` and holds the five
binaries `gonemaster`, `gonemaster-server`, `gonemaster-client`,
`gonemaster-nagios`, and `gonemaster-mcp`, plus `LICENSE` and `Changelog`. The
server binary in the archive embeds the admin, public, and analysis UIs.

`checksums.txt` lists the SHA-256 sum of every attached file, with no directory
component. Verify before installing:

    sha256sum --ignore-missing -c checksums.txt

Attachments are kept on the three most recent releases and stripped from older
ones. Release notes remain. A download link MUST therefore point at a current
release rather than an arbitrary older tag.

A Forgejo package registry hosted on Codeberg is planned. Once enabled, it
serves `apt install` and `dnf install` from a registry URL without manual
download.

## Install

### Debian / Ubuntu

    # Download the .deb files for your architecture from the release page.
    sudo apt install ./gonemaster-server_1.7.9_amd64.deb

Add `./gonemaster_1.7.9_amd64.deb` and others as needed. `apt` lists
`gonemaster-badkeys-data` as a recommended package and proceeds without it.

### Fedora / RHEL / Rocky

    sudo dnf install ./gonemaster-server-1.7.9-1.x86_64.rpm

`dnf` treats `gonemaster-badkeys-data` as a weak dependency and proceeds
without it.

### Binary archive

    tar xzf gonemaster_v1.7.9_linux_amd64.tar.gz
    sudo install -m 0755 gonemaster-server /usr/local/bin/

The archive carries no systemd unit, env file, or man pages. Installing from a
package is required for a service install.

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

## Blocklist data

The badkeys blocklist is not attached to releases. Without it, dnssec19 reports
`DS19_BLOCKLIST_NOT_FOUND` and checks no keys against the blocklist. Every
other test runs unchanged.

Fetch the data with the CLI:

    gonemaster --badkeys-update

The files land in `~/.local/share/gonemaster/badkeys/`, which the binaries
search before the system directories. For a system-wide install, write them to
a directory on `XDG_DATA_DIRS`:

    sudo gonemaster --badkeys-update --badkeys-path /usr/share/gonemaster/badkeys

Refresh at whatever interval suits the deployment. The data is independent of
the binaries.

Building `gonemaster-badkeys-data` from a source checkout with `make packages`
produces a package that installs the same two files.

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

The blocklist updates separately; see [Blocklist data](#blocklist-data).

## Uninstall

    # Debian/Ubuntu
    sudo apt remove gonemaster-server
    sudo apt purge gonemaster-server     # additionally wipes /etc/gonemaster

    # Fedora/RHEL
    sudo dnf remove gonemaster-server

`remove` leaves the `gonemaster` system user and `/var/lib/gonemaster/` in
place so an accidental remove-and-reinstall keeps the database. To wipe
everything:

    sudo userdel gonemaster
    sudo groupdel gonemaster
    sudo rm -rf /var/lib/gonemaster /etc/gonemaster
