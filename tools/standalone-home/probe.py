"""Probe the configured local endpoint, without sending appliance commands."""
import base64
import hashlib
import hmac
import json
import os
import time
from urllib.error import HTTPError
from urllib.request import Request, urlopen

from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from echo_home import ROOT
from hall_light import appliances, executor


def request(op, extra=None, endpoint="echo-home"):
    key = bytes.fromhex(json.loads((ROOT / "home-local.json").read_text())["key"])
    box, nonce = AESGCM(key), os.urandom(12)
    data = dict(op=op, time=time.time(), **(extra or {}))
    packet = base64.b64encode(nonce + box.encrypt(nonce, json.dumps(data).encode(), b"echo-home-request-1"))
    req = Request(f"http://127.0.0.1:50226/{endpoint}/v1", data=packet,
                  headers={"X-Echo-Signature": hmac.new(key, packet, hashlib.sha256).hexdigest()})
    with urlopen(req, timeout=85) as res:
        raw = base64.b64decode(res.read())
    return json.loads(box.decrypt(raw[:12], raw[12:], b"echo-home-response-1" + nonce))


if __name__ == "__main__":
    env = request("environment")
    assert env["ok"] and env["environment"]
    print("Sensor fields:", sorted(env["environment"]))
    power = request("power_status")
    assert power["ok"] and power["controls"]
    print("Power buttons:", [c["id"] for c in power["controls"]])
    agenda = request("agenda")
    assert agenda["ok"] and not agenda["stale"]
    print("Google feeds:", len(json.loads((ROOT / "home-local.json").read_text())["calendars"]),
          "fresh events:", len(agenda["events"]), "fetched:", agenda["fetched"])
    print("Hall bulb:", executor().status())
    api = appliances()
    checked = [api.resolve(target, "open" if target == "curtain" else "on") for target in api.cfg["voice_devices"]]
    checked += [api.resolve(target, "toggle") for target in api.cfg["voice_scenes"]]
    print("Additional voice targets:", len(checked))
    idle = {k.lower(): v for k, v in json.loads((ROOT.parent / "idle-home.json").read_text()).items()}
    if (ROOT / "enabled").exists():
        assert idle["url"] == "http://127.0.0.1:50226/echo-home/v1"
        brain = json.loads((ROOT.parent / "state.json").read_text())["brain"]
        assert brain["mode"] == "muse" and brain["stt"] == "127.0.0.1:50024"
        for name in ("switchbot.env", "calendar-feeds.txt", "home-local.json"):
            assert (ROOT / name).stat().st_mode & 0o077 == 0
        for text in ("廊下の消灯を受け付けました。", "今日の予定はありません。", "明日の予定は1件です。画面に表示します。"):
            pcm = (ROOT / "voice" / (hashlib.sha256(text.encode()).hexdigest() + ".pcm")).read_bytes()
            assert len(pcm) > 3200 and len(pcm) % 2 == 0 and any(pcm)
        print("Local home endpoint, speech recognition and recorded replies verified.")
    try:
        request("appliance", dict(id="test", target="invalid", direction="on"), endpoint="echo-hall")
    except HTTPError as e:
        assert e.code in (400, 503)
    else:
        raise AssertionError("invalid command accepted")
