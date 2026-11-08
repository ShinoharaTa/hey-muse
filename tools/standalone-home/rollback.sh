#!/bin/sh
set -eu
umask 077
d=/data/misc/techo5/standalone-home
cd "$d"
slot=$(cat /run/techo5/slot)
case "$slot" in a|b) ;; *) exit 1;; esac
root=/store/slots/$slot
test -s inittab.before && test -s techo5.before && test -s idle-home.json.before
touch /run/techo5/hold
trap 'slotctl ro; rm -f /run/techo5/hold' EXIT
slotctl rw
killall techo5 || :
cp -p techo5.before "$root/usr/local/bin/techo5.restore"
mv "$root/usr/local/bin/techo5.restore" "$root/usr/local/bin/techo5"
cp -p inittab.before "$root/etc/inittab.restore"
mv "$root/etc/inittab.restore" "$root/etc/inittab"
cp -p idle-home.json.before ../idle-home.restore
mv ../idle-home.restore ../idle-home.json
rm -f enabled
kill -HUP 1
sync
