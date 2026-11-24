#!/usr/bin/env python3
"""summary.py TIER LOG EXPECTED TIMES: the one table gates/all.sh prints.

Reads every line of LOG that starts with "GATE gate subject PASS|FAIL detail", and
EXPECTED (gates/expected.txt: "gate subject | why", the gates known to fail today).
A gate listed there that fails is shown as XFAIL and does not fail the run; one that
passes is shown as XPASS and does fail it, until its line is removed: the list can only
shrink. A step that printed no GATE line at all fails the run. Exit 0 = the state is as
recorded. Standard library only.

A Go gate that cannot run yet because the Go package lacks a binding prints PENDING; it is
shown as PENDING and passes the run only when gates/go-pending.txt (beside EXPECTED, the same
format) lists it. A listed gate that passes fails the run until its line is removed."""
import os
import re
import sys

tier, log, expected, times = sys.argv[1:5]
known = {}
for line in open(expected):
    line = line.strip()
    if line and not line.startswith("#"):
        key, _, why = line.partition("|")
        known[" ".join(key.split())] = why.strip()
pending = {}
pfile = os.path.join(os.path.dirname(os.path.abspath(expected)), "go-pending.txt")
for line in open(pfile) if os.path.exists(pfile) else []:
    line = line.strip()
    if line and not line.startswith("#"):
        key, _, why = line.partition("|")
        pending[" ".join(key.split())] = why.strip().replace(" | ", "; who: ")
rows, bad, steps = [], [], {}
step = None
for line in open(log, errors="replace"):
    m = re.match(r"== (\S+): ", line)
    if m:
        step = m.group(1)
        steps[step] = 0
    m = re.match(r"GATE (\S+) (\S+) (PASS|FAIL|NONE|PENDING) ?(.*)", line)
    if m:
        gate, subject, result, detail = m.groups()
        key = f"{gate} {subject}"
        if result == "PENDING" and key not in pending:
            result, detail = "FAIL", f"pending but not listed in gates/go-pending.txt. {detail}"
        elif result == "PENDING":
            detail = f"waits for a Go binding: {pending[key]}. {detail}"
        elif key in pending and result == "PASS":
            result, detail = "XPASS", f"runs now: remove it from gates/go-pending.txt. {detail}"
        elif key in known and result == "FAIL":
            result, detail = "XFAIL", f"expected: {known[key]}. {detail}"
        elif key in known and result == "PASS":
            result, detail = "XPASS", f"passes now: remove it from gates/expected.txt. {detail}"
        if result in ("FAIL", "XPASS", "NONE"):
            bad.append(key)
        rows.append((gate, subject, result, detail))
        if step:
            steps[step] += 1
secs = dict((l.split()[0], float(l.split()[1])) for l in open(times) if l.strip())
for s, n in steps.items():
    if n == 0 and s != "build":
        rows.append((s, "-", "FAIL", "the step printed no verdict (see gates/out/all.log)"))
        bad.append(s)
count = lambda r: sum(1 for x in rows if x[2] == r)
judged = next((l.strip() for l in open(log, errors="replace") if l.startswith("judged: ")), "")
print(f"Tier {tier}: {count('PASS')} passed, {count('FAIL') + count('NONE')} failed, {count('XFAIL')} failing as expected, "
      f"{count('XPASS')} passing unexpectedly, {count('PENDING')} pending a Go binding; {secs.get('all', 0):.0f} s in all."
      + (f" {judged[0].upper()}{judged[1:]}." if judged else ""))
print()
print("| gate | subject | result | detail |")
print("|---|---|---|---|")
for r in rows:
    print("| " + " | ".join(r) + " |")
print()
print("Seconds per step: " + ", ".join(f"{k} {v:.1f}" for k, v in secs.items() if k != "all") + ".")
sys.exit(1 if bad or not rows else 0)
