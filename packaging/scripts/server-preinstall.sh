#!/bin/sh
set -e

if ! getent group gonemaster >/dev/null 2>&1; then
    groupadd -r gonemaster
fi

if ! getent passwd gonemaster >/dev/null 2>&1; then
    useradd -r -g gonemaster \
            -d /var/lib/gonemaster \
            -s /usr/sbin/nologin \
            -c "gonemaster service" \
            gonemaster
fi

exit 0
