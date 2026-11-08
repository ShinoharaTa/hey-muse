#!/bin/sh
set -eu
umask 077
d=/data/misc/techo5/standalone-home
mkdir -p "$d/runtime/etc/apk"
chmod 700 "$d"
cp -r /etc/apk/keys "$d/runtime/etc/apk/"
cp /etc/apk/repositories "$d/runtime/etc/apk/"
apk --root "$d/runtime" --initdb --no-cache --no-scripts add python3 py3-cryptography py3-icalendar py3-dateutil tzdata
