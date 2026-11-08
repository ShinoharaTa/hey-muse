"""Stage files over the serial-guarded Echo Show console without logging secrets."""
import argparse
import hashlib
import io
import json
from pathlib import Path
import secrets
import sqlite3
import sys
import tarfile
import tempfile
import time
import zipfile

BASE = Path.home() / "Documents/muse_on_echo"
HERE = Path(__file__).resolve().parent
DEVICE = "/data/misc/techo5/standalone-home"
sys.path.insert(0, str(BASE / "assets/hey-muse/tools"))
from techo5lib import Console, CONSOLE_TECHO5, SerialPort


def send(console, archive):
    assert console.find(), console.waiting_hint()
    size = archive.seek(0, 2)
    archive.seek(0)
    digest = hashlib.file_digest(archive, "sha256").hexdigest()
    archive.seek(0)
    port = SerialPort(console.port)

    def write(data):
        while data:
            count = port._wintypes.DWORD()
            if not port.k32.WriteFile(port.handle, data, len(data), port._ctypes.byref(count), None) or not count.value:
                raise OSError("USB write failed")
            data = data[count.value:]

    def until(marker, seconds):
        buf = b""
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            buf += port.read()
            if marker in buf:
                return buf
            time.sleep(.002)
        raise TimeoutError("Echo USB transfer timed out")

    tag = secrets.token_hex(6)
    try:
        port.write(b"\n")
        time.sleep(.2)
        while port.read():
            pass
        # Stage inside a root-only directory. Serial data is never echoed.
        cmd = (f"umask 077; mode=$(stty -g); stty raw -echo; printf '\\nR{tag}\\n'; "
               f"head -c {size} > {DEVICE}/stage.tar.gz; stty \"$mode\"; "
               f"sha256sum {DEVICE}/stage.tar.gz; printf '\\nD{tag}\\n'\n")
        write(cmd.encode())
        until(f"\nR{tag}\n".encode(), 8)
        while chunk := archive.read(32768):
            write(chunk)
        if digest.encode() not in until(f"D{tag}".encode(), 90):
            raise ValueError("USB checksum mismatch")
    finally:
        port.close()


def deploy(args):
    console = Console(args.serial, CONSOLE_TECHO5)
    def run(command, timeout=30):
        reply = console.run(f"({command}) && echo HOME_OK", timeout)
        if reply is None or "HOME_OK" not in reply.splitlines():
            raise RuntimeError("Echo command failed: " + str(reply))

    run(f"umask 077; mkdir -p {DEVICE}; chmod 700 {DEVICE}")
    # Spool secrets only in memory. The larger binary/audio archive contains no credentials.
    file = io.BytesIO() if args.part == "service" else tempfile.TemporaryFile()
    with file as archive, tarfile.open(fileobj=archive, mode="w:gz", compresslevel=2) as tar:
        def add(name, content, mode=0o600):
            info = tarfile.TarInfo(name)
            info.mode, info.size = mode, len(content)
            tar.addfile(info, io.BytesIO(content))

        if args.part == "service":
            bridge = BASE / "speech-bridge"
            sys.path.insert(0, str(bridge))
            import echo_home as live
            import hall_light as light
            cfg = json.loads((bridge / "home-local.json").read_text(encoding="utf-8-sig"))
            cfg.pop("codex", None)
            cfg["power_controls"] = {k: dict(label=v[0], icon=v[1], toggle=v[2], on=v[3], off=v[4])
                                     for k, v in live.POWER_NAMES.items()}
            cfg["voice_devices"] = {k: dict(name=v[0], type=v[1]) for k, v in light.VOICE_DEVICES.items()}
            cfg["voice_scenes"] = {k: {op: name} for (k, op), name in light.VOICE_SCENES.items()}
            cfg["hall"] = dict(name="廊下の電気", type="Color Bulb", transport="cloud")
            add("home-local.json", json.dumps(cfg, ensure_ascii=False).encode())
            add("switchbot.env", (Path.home() / "switchbot_token.env").read_bytes())
            add("calendar-feeds.txt", (BASE / "calendar-feeds.txt").read_bytes())
            with sqlite3.connect(f"file:{(bridge / 'home-requests.sqlite').as_posix()}?mode=ro", uri=True) as db:
                add("initial-power.json", json.dumps(dict(db.execute("select device,direction from power_last"))).encode())
            for name in ("echo_home.py", "hall_light.py"):
                add(name, (args.repo / "tools/mothership" / name).read_bytes())
            for name in ("calendar_feed.py", "server.py", "probe.py", "home-python", "enable.py"):
                add(name, (HERE / name).read_bytes().replace(b"\r\n", b"\n"), 0o700)
            for wheel in sorted(args.wheels.glob("*.whl")):
                if not wheel.name.endswith("none-any.whl"):
                    raise ValueError("expected pure Python wheels")
                with zipfile.ZipFile(wheel) as z:
                    for member in z.infolist():
                        if not member.is_dir() and ".data/" not in member.filename:
                            add("vendor/" + member.filename, z.read(member))
        else:
            add("techo5.next", args.binary.read_bytes(), 0o755)
            for name in ("install.sh", "rollback.sh", "enable.py"):
                add(name, (HERE / name).read_bytes().replace(b"\r\n", b"\n"), 0o700)
            files = list(args.audio.glob("*.pcm"))
            assert len(files) >= 130
            for path in files:
                add("voice/" + path.name, path.read_bytes())
        tar.close()
        send(console, archive)
        run(f"cd {DEVICE} && tar xzf stage.tar.gz && rm -f stage.tar.gz && sync", 65)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("part", choices=("service", "daemon"))
    parser.add_argument("--serial", required=True)
    for flag in ("repo", "wheels", "audio", "binary"):
        parser.add_argument("--" + flag, type=Path)
    deploy(parser.parse_args())
    print("Staged and verified.")
