#!/bin/sh
set -e

if [ -d /run/systemd/system ]; then
    systemctl daemon-reload >/dev/null 2>&1 || true
fi

# The gonemaster system user and /var/lib/gonemaster are intentionally left
# in place so an accidental remove/reinstall preserves the database. A full
# wipe (deb purge) is the operator's responsibility.

exit 0
