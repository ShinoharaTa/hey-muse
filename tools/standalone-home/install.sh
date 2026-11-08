#!/bin/sh
set -eu
umask 077
d=/data/misc/techo5/standalone-home
cd "$d"
slot=$(cat /run/techo5/slot)
case "$slot" in a|b) ;; *) exit 1;; esac
test "$slot" = "$(cat /store/active)"
root=/store/slots/$slot
test -x ./techo5.next
./techo5.next --version >/dev/null
./home-python enable.py prepare
test ! -e /run/techo5/hold
entry="::respawn:$d/home-python $d/server.py"
# Backups precede all configuration changes, and survive a retry.
for pair in "inittab /etc/inittab" "techo5 /usr/local/bin/techo5" "idle-home.json /data/misc/techo5/idle-home.json"; do
    set -- $pair
    if ! test -f "$1.before"; then
        cp -p "$2" "$1.before.new"
        mv "$1.before.new" "$1.before"
    fi
done
recover() {
    rc=$?
    trap - EXIT
    if [ "$rc" -ne 0 ]; then
        sh "$d/rollback.sh"
    else
        slotctl ro
        rm -f /run/techo5/hold
    fi
    exit "$rc"
}
trap recover EXIT
slotctl rw
if ! grep -Fqx "$entry" "$root/etc/inittab"; then
    cp -p "$root/etc/inittab" "$root/etc/inittab.new"
    printf '\n%s\n' "$entry" >> "$root/etc/inittab.new"
    mv "$root/etc/inittab.new" "$root/etc/inittab"
fi
if [ -f /run/standalone-test.pid ]; then
    old=$(cat /run/standalone-test.pid)
    if [ -e "/proc/$old/cmdline" ] && tr '\000' ' ' < "/proc/$old/cmdline" | grep -Fq 'server.py'; then
        kill "$old"
    fi
    rm -f /run/standalone-test.pid
fi
kill -HUP 1
i=0
until netstat -lnt | grep -q '127.0.0.1:50226 '; do
    i=$((i+1)); [ "$i" -lt 30 ]; sleep 1
done
touch /run/techo5/hold
killall techo5 || :
sleep 2
test -z "$(pidof techo5)"
cp ./techo5.next "$root/usr/local/bin/techo5.new"
chmod 755 "$root/usr/local/bin/techo5.new"
mv "$root/usr/local/bin/techo5.new" "$root/usr/local/bin/techo5"
./home-python enable.py switch
sync
slotctl ro
rm -f /run/techo5/hold
i=0
until pidof techo5 >/dev/null; do
    i=$((i+1)); [ "$i" -lt 40 ]; sleep 1
done
pid=$(pidof techo5)
./home-python probe.py
sleep 5
test "$pid" = "$(pidof techo5)"
echo STANDALONE_INSTALLED
