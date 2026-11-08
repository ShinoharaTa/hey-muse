"""Prepare the fixed Japanese home-control replies with the owner's VOICEVOX engine."""
import argparse
import hashlib
import io
import json
from pathlib import Path
from urllib.parse import urlencode
from urllib.request import Request, urlopen
import wave


def replies():
    labels = ["廊下の点灯", "廊下の消灯", "モニターの電源切替", "プロジェクターの電源切替",
              "カーテンの開く操作", "カーテンの閉じる操作"]
    for device in ("エアコン", "リビングの照明", "プラグミニ", "掃除機", "サーキュレーター"):
        labels.extend(device + "の" + action for action in ("オン", "オフ"))
    for label in labels:
        for suffix in ("を受け付けました。", "を受け付けたか確認できませんでした。"):
            yield label + suffix
    yield "予定を取得できていません。画面で取得状況を確認してください。"
    yield "取得済みの予定を画面に表示します。"
    yield "聞き取れませんでした。もう一度お願いします。"
    yield "操作を見送りました。"
    for title in ("今日の予定", "明日の予定", "今週の予定", "7日間の予定"):
        yield title + "はありません。"
        for count in range(1, 24):
            yield f"{title}は{count}件です。画面に表示します。"


def prepare(directory, port):
    directory.mkdir(parents=True, exist_ok=True)
    base = f"http://127.0.0.1:{port}"
    for text in replies():
        path = directory / (hashlib.sha256(text.encode()).hexdigest() + ".pcm")
        if path.exists():
            continue
        with urlopen(Request(base + "/audio_query?" + urlencode(dict(text=text, speaker=3)), data=b""), timeout=60) as res:
            query = json.load(res)
        query.update(outputSamplingRate=16000, outputStereo=False)
        req = Request(base + "/synthesis?speaker=3", data=json.dumps(query).encode(),
                      headers={"Content-Type": "application/json"})
        with urlopen(req, timeout=60) as res, wave.open(io.BytesIO(res.read())) as wav:
            assert (wav.getframerate(), wav.getsampwidth(), wav.getnchannels()) == (16000, 2, 1)
            pcm = wav.readframes(wav.getnframes())
            assert len(pcm) == 2 * wav.getnframes() and len(pcm) > 0
        tmp = path.with_suffix(".tmp")
        tmp.write_bytes(pcm)
        tmp.replace(path)
    print("Prepared", len(list(directory.glob("*.pcm"))), "replies.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("directory", type=Path)
    parser.add_argument("--port", type=int, default=50021)
    args = parser.parse_args()
    prepare(args.directory, args.port)
