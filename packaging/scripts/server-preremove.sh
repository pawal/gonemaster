#!/bin/sh
set -e

# Stop and disable only on actual removal, not on upgrade.
#   deb: $1 = "remove" or "purge" on uninstall, "upgrade" on upgrade.
#   rpm: $1 = 0 on uninstall, 1 on upgrade.
case "$1" in
    remove|purge|0)
        if [ -d /run/systemd/system ]; then
            systemctl stop gonemaster-server.service >/dev/null 2>&1 || true
            systemctl disable gonemaster-server.service >/dev/null 2>&1 || true
        fi
        ;;
esac

exit 0
