"""Read Google Calendar iCal subscriptions and retain the last successful agenda."""
import json
import os
import re
from datetime import date, datetime, time, timedelta
from urllib.parse import urlsplit
from urllib.request import HTTPRedirectHandler, build_opener
from zoneinfo import ZoneInfo

import icalendar
import recurring_ical_events

JST = ZoneInfo("Asia/Tokyo")
MAX_BYTES = 20 * 1024 * 1024


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, *args):
        return None


def google_feed(url):
    u = urlsplit(url)
    if (u.scheme != "https" or u.netloc != "calendar.google.com" or u.query or u.fragment
            or not re.fullmatch(r"/calendar/ical/[^/]+/(?:private-[\w-]+|public)/basic\.ics", u.path)):
        raise ValueError("invalid Google Calendar feed")
    with build_opener(NoRedirect()).open(url, timeout=18) as response:
        body = response.read(MAX_BYTES + 1)
    if len(body) > MAX_BYTES or not body.lstrip().upper().startswith(b"BEGIN:VCALENDAR"):
        raise ValueError("invalid calendar response")
    return body


def local(value):
    if not isinstance(value, datetime):
        return datetime.combine(value, time.min, JST)
    return value.replace(tzinfo=JST) if value.tzinfo is None else value.astimezone(JST)


def expand(data, name, now, until):
    calendar = icalendar.Calendar.from_ical(data)
    rows = []
    for event in recurring_ical_events.of(calendar).between(now, until):
        if str(event.get("STATUS", "")).upper() == "CANCELLED":
            continue
        start = event.start
        begin, end = local(start), local(event.end)
        if begin < until and (end > now or end == begin and begin >= now):
            rows.append(dict(title=str(event.get("SUMMARY") or "予定")[:160],
                             start=begin.isoformat(), end=end.isoformat(),
                             day=type(start) is date, calendar=name))
    return rows


def save(path, data):
    tmp = path.with_suffix(".tmp")
    tmp.write_text(json.dumps(data, ensure_ascii=False), encoding="utf-8")
    os.replace(tmp, path)


def agenda(root, calendars):
    now = datetime.now(JST)
    feeds_path = root / "calendar-feeds.txt"
    cache_path = root / "agenda-cache.json"
    stale = True
    if feeds_path.exists():
        try:
            urls = feeds_path.read_text(encoding="utf-8-sig").split()
            if len(urls) != len(calendars) or len(set(urls)) != len(urls):
                raise ValueError("one feed required per calendar")
            until = now + timedelta(days=8)
            rows = []
            for url, cal in zip(urls, calendars, strict=True):
                rows.extend(expand(google_feed(url), cal["name"], now, until))
            data = dict(events=rows, updated=now.isoformat(), until=until.isoformat())
            save(cache_path, data)
            stale = False
        except Exception:
            pass  # Keep the previous complete snapshot, including on a partial refresh.
    if stale:
        if not cache_path.exists():
            return dict(ok=False)
        data = json.loads(cache_path.read_text(encoding="utf-8"))
        if datetime.fromisoformat(data["until"]) <= now:
            return dict(ok=False)
    end = now + timedelta(days=7)
    rows = [e for e in data["events"] if datetime.fromisoformat(e["end"]) > now
            and datetime.fromisoformat(e["start"]) < end]
    rows.sort(key=lambda e: datetime.fromisoformat(e["start"]))
    return dict(ok=True, events=rows[:24], stale=stale, fetched=data["updated"])
