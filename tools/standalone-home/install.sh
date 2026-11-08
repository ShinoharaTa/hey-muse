#!/bin/sh
set -eu
umask 077
d=/data/misc/techo5/standalone-home
cd "$d"
slot=$(cat /run/techo5/slot)
case "$slot" in a|b) ;; *) exit 1;; esac
test "$slot" = "$(cat /store/active)"
root=/store/slots/$slot
test ! -e /run/techo5/hold
for f in techo5.next home-local.json switchbot.env calendar-feeds.txt ../idle-home.json; do test -s "$f"; done
test -d voice
./techo5.next --version >/dev/null

# Keep the original installation for the user's rollback, and the immediately
# preceding version for recovery if this particular upgrade is interrupted.
for pair in "inittab /etc/inittab" "techo5 /usr/local/bin/techo5" "idle-home.json ../idle-home.json"; do
    set -- $pair
    if ! test -s "$1.before"; then
        cp -p "$2" "$1.before.new"
        mv "$1.before.new" "$1.before"
    fi
done

backup=$(mktemp -d "$d/upgrade.XXXXXX")
cp -p /etc/inittab "$backup/inittab"
cp -p /usr/local/bin/techo5 "$backup/techo5"
if test -f enabled; then touch "$backup/enabled"; fi
if test -f power-last.json; then cp -p power-last.json "$backup/power-last.json"; fi

finish() {
    rc=$?
    trap - EXIT
    if test "$rc" -ne 0; then
        touch /run/techo5/hold
        killall techo5 2>/dev/null || :
        slotctl rw
        cp -p "$backup/techo5" "$root/usr/local/bin/techo5.restore"
        mv "$root/usr/local/bin/techo5.restore" "$root/usr/local/bin/techo5"
        cp -p "$backup/inittab" "$root/etc/inittab.restore"
        mv "$root/etc/inittab.restore" "$root/etc/inittab"
        if test ! -f "$backup/enabled"; then rm -f enabled; fi
        if test -f "$backup/power-last.json"; then cp -p "$backup/power-last.json" power-last.json; else rm -f power-last.json; fi
        kill -HUP 1
    fi
    slotctl ro
    rm -f /run/techo5/hold
    rm -f "$backup/techo5" "$backup/inittab" "$backup/power-last.json" "$backup/enabled"
    rmdir "$backup"
    exit "$rc"
}
trap finish EXIT
slotctl rw
touch /run/techo5/hold
killall techo5 2>/dev/null || :
sleep 2
test -z "$(pidof techo5)"

entry="::respawn:$d/home-python $d/server.py"
if grep -Fqx "$entry" /etc/inittab; then
    cp -p /etc/inittab "$root/etc/inittab.new"
    grep -Fvx "$entry" /etc/inittab > "$root/etc/inittab.new"
    mv "$root/etc/inittab.new" "$root/etc/inittab"
    kill -HUP 1
    sleep 1
    # Some BusyBox builds leave the removed service running until it exits.
    for pid in $(pidof python3 || :); do
        if tr '\000' '\n' < "/proc/$pid/cmdline" | grep -Fxq "$d/server.py"; then kill "$pid"; fi
    done
fi
if test ! -f power-last.json; then
    if test -f home-requests.sqlite; then
        test ! -s home-requests.sqlite-journal
        test ! -s home-requests.sqlite-wal
        sha256sum -c migration-power.sha256
        cp migration-power.json power-last.tmp
    else
        cp initial-power.json power-last.tmp
    fi
    mv power-last.tmp power-last.json
fi
cp techo5.next "$root/usr/local/bin/techo5.new"
chmod 755 "$root/usr/local/bin/techo5.new"
mv "$root/usr/local/bin/techo5.new" "$root/usr/local/bin/techo5"
touch enabled
sync
slotctl ro
rm -f /run/techo5/hold
i=0
until pidof techo5 >/dev/null; do
    i=$((i+1)); test "$i" -lt 40; sleep 1
done
pid=$(pidof techo5)
sleep 5
test "$pid" = "$(pidof techo5)"
echo NATIVE_HOME_INSTALLED
