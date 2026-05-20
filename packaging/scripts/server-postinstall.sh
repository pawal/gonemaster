#!/bin/sh
set -e

# Ensure /var/lib/gonemaster exists with correct ownership.
if [ -x /usr/bin/systemd-tmpfiles ] || [ -x /bin/systemd-tmpfiles ]; then
    systemd-tmpfiles --create /usr/lib/tmpfiles.d/gonemaster.conf >/dev/null 2>&1 || true
else
    mkdir -p /var/lib/gonemaster
    chown gonemaster:gonemaster /var/lib/gonemaster
    chmod 0750 /var/lib/gonemaster
fi

if [ -d /run/systemd/system ]; then
    systemctl daemon-reload >/dev/null 2>&1 || true
fi

# Respect distro preset policy on first install only.
case "$1" in
    configure)
        # deb: $2 is the previous version on upgrade, empty on first install.
        if [ -z "$2" ]; then
            if [ -x /usr/bin/deb-systemd-helper ]; then
                deb-systemd-helper preset gonemaster-server.service >/dev/null 2>&1 || true
            else
                systemctl preset gonemaster-server.service >/dev/null 2>&1 || true
            fi
        fi
        ;;
    1)
        # rpm: 1 = first install.
        systemctl preset gonemaster-server.service >/dev/null 2>&1 || true
        ;;
esac

exit 0
