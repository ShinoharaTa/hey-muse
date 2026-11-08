#!/bin/sh
# Remove only the files staged by the former on-device Python service.
set -eu
d=/data/misc/techo5/standalone-home
cd "$d"
test -f enabled
test -s power-last.json
pid=$(pidof techo5)
test -n "$pid"
cmp -s "/proc/$pid/exe" techo5.next
if grep -Fq "$d/server.py" /etc/inittab; then exit 1; fi
for pid in $(pidof python3 || :); do
    if tr '\000' '\n' < "/proc/$pid/cmdline" | grep -Fxq "$d/server.py"; then exit 1; fi
done
rm -rf "$d/runtime" "$d/vendor" "$d/__pycache__"
rm -f home-python server.py echo_home.py hall_light.py calendar_feed.py enable.py probe.py test_calendar_feed.py
