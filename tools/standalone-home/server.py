"""Authenticated loopback endpoint for the Echo's existing home and voice clients."""
import hashlib
import hmac
import json
import os
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

import calendar_feed
import echo_home
import hall_light

ROOT = Path(__file__).resolve().parent
AGENDA_LOCK = threading.Lock()


def dashboard(packet):
    cfg, cipher, nonce, req = echo_home.unpack(packet)
    op, ident = req["op"], req.get("id", "")
    definitions = cfg.get("power_controls", {})
    result = dict(ok=False)
    if op == "agenda":
        with AGENDA_LOCK:
            result = calendar_feed.agenda(ROOT, cfg["calendars"])
        try:
            result.update(controls=echo_home.power(hall_light.appliances(), definitions)["controls"])
        except Exception:
            pass
        return echo_home.pack(cipher, nonce, result)
    api = hall_light.appliances()
    if op == "environment" and cfg.get("environment_device"):
        item = api.request("/" + cfg["environment_device"] + "/status")
        readings = {k: item[k] for k in ("temperature", "humidity", "CO2", "battery")
                    if type(item.get(k)) in (int, float)}
        result = dict(ok=bool(readings), environment=readings)
    elif op == "power_status":
        result = echo_home.power(api, definitions)
    elif op == "scene" and ident.startswith("power/"):
        result = echo_home.power(api, definitions, ident)
    elif op == "scene" and ident in cfg.get("scenes", []):
        echo_home.execute_scene(api, ident)
        result = dict(ok=True)
    return echo_home.pack(cipher, nonce, result)


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        if self.path not in ("/echo-home/v1", "/echo-hall/v1"):
            return self.send_error(404)
        lengths = self.headers.get_all("Content-Length", [])
        if (self.headers.get("Transfer-Encoding") or len(lengths) != 1 or not lengths[0].isascii()
                or not lengths[0].isdigit() or len(lengths[0]) > 4 or not 1 <= int(lengths[0]) <= 4096):
            return self.send_error(400)
        self.connection.settimeout(5)
        packet = self.rfile.read(int(lengths[0]))
        expected = hmac.new(self.server.key, packet, hashlib.sha256).hexdigest()
        if not hmac.compare_digest(expected, self.headers.get("X-Echo-Signature", "")):
            return self.send_error(403)
        try:
            reply = hall_light.dispatch(packet) if self.path == "/echo-hall/v1" else dashboard(packet)
        except Exception:
            return self.send_error(503)
        self.send_response(200)
        self.send_header("Content-Type", "application/octet-stream")
        self.send_header("Content-Length", str(len(reply)))
        self.end_headers()
        self.wfile.write(reply)


if __name__ == "__main__":
    os.umask(0o077)
    with ThreadingHTTPServer(("127.0.0.1", 50226), Handler) as server:
        server.key = bytes.fromhex(json.loads((ROOT / "home-local.json").read_text())["key"])
        server.serve_forever()
