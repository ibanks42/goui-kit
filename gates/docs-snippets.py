#!/usr/bin/env python3
"""docs-snippets.py DOC...: every Go snippet of the guides is code that compiles.

Each ```go block of a guide must follow a line `<!-- from: PATH -->` naming a program
(gates/docs-go.sh builds and runs every one of them). The block is that file's text: in order, with
lines `// ...` standing for lines left out (a block need not start at the top). Exact lines, tabs
and all, so a snippet cannot drift from the program it was cut from. Prints one line per guide;
exit 1 on the first snippet that is not in its program.
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


def find(lines, chunk, start):
    n = len(chunk)
    for i in range(start, len(lines) - n + 1):
        if lines[i:i + n] == chunk:
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
    """A guide with `<!-- spans: PATH -->` names a program in which every inline code span that is
    a call must appear as text: one that starts with gk., a. or a method's dot (`.OnClick(`) and
    has a parenthesis, without a `..` placeholder. The table
    cells escape | as \\|; the generated index (after its marker) is not checked here."""
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
            continue  # not a call, or one with placeholders (`.Style(..)`)
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
