#!/usr/bin/env python3
"""Generate a deterministic Markdown feed for the reader gates (gates/reader.sh, gates/go-apps.sh).

Usage:
  reader-feed.py <out.md> [count]   Write <count> articles (default 3000) starting from sample-feed.md.
"""
import datetime
import os
import sys

here = os.path.dirname(os.path.abspath(__file__))
sample = open(os.path.join(here, "..", "examples", "go", "reader", "sample-feed.md")).read()
out, n = sys.argv[1], int(sys.argv[2]) if len(sys.argv) > 2 else 3000
SOURCES = ["Field Notes", "Systems Weekly", "Kitchen Table", "Desk Setup", "Project Log", "Night Train"]
TOPICS = ["gardens", "compilers", "rivers", "bicycles", "maps", "teapots", "radios", "lighthouses",
          "notebooks", "bridges", "clocks", "orchards"]
WORDS = ("the quick reader keeps a list of small things worth reading and comes back to them on slow "
         "evenings when the light is low and the kettle is on").split()
parts = [sample.rstrip("\n")]
day = datetime.date(2026, 11, 4)
for i in range(sample.count("+++ "), n):
    topic = TOPICS[i % len(TOPICS)]
    when = day - datetime.timedelta(days=(i // 6))
    para = " ".join(WORDS[(i + k) % len(WORDS)] for k in range(40))
    parts.append(f"+++ On {topic}, part {i} | {SOURCES[i % len(SOURCES)]} | {when.isoformat()}\n"
                 f"# On {topic}, part {i}\n\n{para.capitalize()}.\n\n"
                 f"- one thing about {topic}\n- another thing\n\n> number {i} of {n}\n")
open(out, "w").write("\n".join(parts) + "\n")
