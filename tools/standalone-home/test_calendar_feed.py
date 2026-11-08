import json
from datetime import datetime, timedelta
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import calendar_feed as feed


class CalendarTest(unittest.TestCase):
    def test_recurring_exception_and_japanese_all_day(self):
        ics = b"""BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Echo//EN
BEGIN:VEVENT
UID:slots
DTSTART:20261106T233000Z
DTEND:20261107T000000Z
RRULE:FREQ=DAILY;COUNT=4
EXDATE:20261107T233000Z
SUMMARY:Slot
END:VEVENT
BEGIN:VEVENT
UID:slots
RECURRENCE-ID:20261108T233000Z
DTSTART:20261109T023000Z
DTEND:20261109T030000Z
SUMMARY:Moved
END:VEVENT
BEGIN:VEVENT
UID:day
DTSTART;VALUE=DATE:20261109
DTEND;VALUE=DATE:20261110
SUMMARY:Day
END:VEVENT
END:VCALENDAR"""
        start = datetime(2026, 11, 8, tzinfo=feed.JST)
        rows = feed.expand(ics, "test", start, start + timedelta(days=2))
        self.assertEqual(sorted((r["title"], r["start"], r["day"]) for r in rows),
                         [("Day", "2026-11-09T00:00:00+09:00", True),
                          ("Moved", "2026-11-09T11:30:00+09:00", False)])

    def test_partial_refresh_preserves_complete_cache(self):
        now = datetime.now(feed.JST)
        cache = dict(events=[], updated=now.isoformat(), until=(now + timedelta(days=8)).isoformat())
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            path = root / "agenda-cache.json"
            feed.save(path, cache)
            (root / "calendar-feeds.txt").write_text("one\ntwo")
            with patch.object(feed, "google_feed", side_effect=[b"first", OSError()]), patch.object(feed, "expand", return_value=[]):
                resp = feed.agenda(root, [{"name": "one"}, {"name": "two"}])
            self.assertTrue(resp["ok"] and resp["stale"])
            self.assertEqual(json.loads(path.read_text()), cache)
