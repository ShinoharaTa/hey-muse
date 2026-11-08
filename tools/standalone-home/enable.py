"""Initialize device-local state and switch the existing home screen to loopback."""
import json
import os
from pathlib import Path
import sqlite3
import sys

ROOT = Path(__file__).resolve().parent
IDLE = ROOT.parent / "idle-home.json"
URL = "http://127.0.0.1:50226/echo-home/v1"


def prepare():
    from calendar_feed import google_feed
    from cryptography.hazmat.primitives.ciphers.aead import AESGCM
    cfg = json.loads((ROOT / "home-local.json").read_text())
    current = {k.lower(): v for k, v in json.loads(IDLE.read_text()).items()}
    assert cfg["key"] == current["key"]
    assert len((ROOT / "calendar-feeds.txt").read_text(encoding="utf-8-sig").split()) == len(cfg["calendars"])
    with sqlite3.connect(ROOT / "home-requests.sqlite") as db:
        db.execute("create table if not exists power_last (device text primary key, direction text)")
        db.executemany("insert or ignore into power_last values (?, ?)",
                       json.loads((ROOT / "initial-power.json").read_text()).items())


def switch():
    current = json.loads(IDLE.read_text())
    old = ROOT / "idle-home.json.before"
    url_key = next(k for k in current if k.lower() == "url")
    assert old.is_file()
    current[url_key] = URL
    tmp = ROOT.parent / "idle-home.standalone.tmp"
    tmp.write_text(json.dumps(current, ensure_ascii=False), encoding="utf-8")
    os.chmod(tmp, 0o600)
    os.replace(tmp, IDLE)
    (ROOT / "enabled").touch(mode=0o600)


if __name__ == "__main__":
    os.umask(0o077)
    {"prepare": prepare, "switch": switch}[sys.argv[1]]()
