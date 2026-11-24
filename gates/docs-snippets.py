#!/usr/bin/env python3
"""Verify that Go code blocks and inline API call spans in Markdown guides match real programs.

Gates: every fenced ```go block and inline backticked API call in the given Markdown files.
Method:
- Each ```go fence must be preceded within 1-2 lines by `<!-- from: PATH -->`. Chunks split by
  `// ...` must appear verbatim and in order inside PATH.
- If a file contains `<!-- spans: PATH -->`, every inline backticked call matching `gk.`, `a.`, or
  `.Capitalized(` before `<!-- index:begin` (excluding `..` and ellipsis) must occur in PATH.
PASS: every code block and inline span is found in its source program (exit 0).
"""
import os
import re
import sys

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
FROM = re.compile(r"^<!-- from: (\S+) -->$")
SPANS = re.compile(r"^<!-- spans: (\S+) -->$", re.M)
SPAN = re.compile(r"`([^`\n]+)`")
INDEX = "<!-- index:begin"

def chunks(block):
    out, cur = [], []
    for line in block:
        if line.strip() == "// ...":
            if cur:
                out.append(cur)
            cur = []
        else:
            cur.append(line)
    if cur:
        out.append(cur)
    return out

def dedent(lines):
    lines = [l.rstrip() for l in lines]
    pads = [len(l) - len(l.lstrip()) for l in lines if l]
    cut = min(pads) if pads else 0
    return [l[cut:] for l in lines]

def find(lines, chunk, start):
    n = len(chunk)
    want = dedent(chunk)
    for i in range(start, len(lines) - n + 1):
        if dedent(lines[i:i + n]) == want:
            return i + n
    return -1

def check(doc):
    text = open(doc).read().split("\n")
    blocks, bad = 0, []
    i = 0
    while i < len(text):
        if text[i].strip() == "```go":
            lang = text[i].strip()[3:]
            src = None
            for back in (1, 2):
                if i - back >= 0 and FROM.match(text[i - back].strip()):
                    src = FROM.match(text[i - back].strip()).group(1)
                    break
            j = i + 1
            while j < len(text) and text[j].strip() != "```":
                j += 1
            block = text[i + 1:j]
            blocks += 1
            where = f"{os.path.relpath(doc, REPO)}:{i + 1}"
            if src is None:
                bad.append(f"{where}: an {lang} block without <!-- from: PATH --> before it")
            elif not os.path.isfile(os.path.join(REPO, src)):
                bad.append(f"{where}: {src} does not exist")
            else:
                lines = open(os.path.join(REPO, src)).read().split("\n")
                at = 0
                for c in chunks(block):
                    at = find(lines, c, at)
                    if at < 0:
                        bad.append(f"{where}: not in {src} (from the line {c[0].strip()[:60]!r})")
                        break
            i = j
        i += 1
    return blocks, bad

def check_spans(doc):
    text = open(doc).read()
    m = SPANS.search(text)
    if not m:
        return 0, []
    prog = open(os.path.join(REPO, m.group(1))).read()
    text = text.split(INDEX)[0]
    bad, n = [], 0
    for span in SPAN.findall(text):
        span = span.replace("\\|", "|")
        if not re.match(r"(gk\.|a\.|\.[A-Z])", span) or "(" not in span or ".." in span or "…" in span:
            continue
        n += 1
        if span not in prog:
            bad.append(f"{os.path.relpath(doc, REPO)}: `{span}` is not text of {m.group(1)}")
    return n, bad

def main(docs):
    rc = 0
    for d in docs:
        n, bad = check(d)
        if bad:
            rc = 1
            for b in bad:
                print("docs-snippets: " + b)
        print(f"docs-snippets: {os.path.relpath(d, REPO)}: {n} code blocks, {n - len(bad)} found in their programs")
        n, bad = check_spans(d)
        if n or bad:
            rc = rc or (1 if bad else 0)
            for b in bad:
                print("docs-snippets: " + b)
            print(f"docs-snippets: {os.path.relpath(d, REPO)}: {n} calls in the text, {n - len(bad)} found in the program")
    return rc

if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
