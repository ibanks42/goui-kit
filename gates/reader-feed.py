#!/usr/bin/env python3
"""reader-feed.py OUT [N]: writes a feed file of N articles (default 3000) for the reader example
(gates/reader.sh): the bundled sample's articles first, then generated ones, one per hour going
back from the sample's dates. Deterministic: the same file every run."""
import datetime
import os
import sys

here = os.path.dirname(os.path.abspath(__file__))
sample = open(os.path.join(here, "..", "examples", "odin", "reader", "sample-feed.md")).read()
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
