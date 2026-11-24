#!/usr/bin/env python3
"""Verify the JSON state file saved by the todo app after todo.script and todo-reload.script.

Usage:
  todo-check.py <todo.json>

PASS: all 7 assertions on projects, Inbox tasks, Work tasks, sync task, and theme flag succeed.
"""
import json
import sys

s = json.load(open(sys.argv[1]))
projects = {p["name"]: p for p in s["projects"]}
inbox = {t["title"]: t for t in projects.get("Inbox", {}).get("tasks") or []}
checks = [
    ("four projects, Garden added through the input", list(projects) == ["Inbox", "Work", "Home", "Garden"]),
    ("Buy milk ticked", inbox.get("Buy milk", {}).get("done") is True),
    ("Call the plumber deleted", "Call the plumber" not in inbox),
    ("Water the plants added, due 2026-11-12, High, open",
     inbox.get("Water the plants") == {"title": "Water the plants", "done": False, "due": "2026-11-12", "priority": "High"}),
    ("the sync thread's task in the Inbox", "Reply to the sync survey" in inbox),
    ("Work untouched", [t["title"] for t in projects["Work"]["tasks"]] == ["Write the quarterly report", "Review Dana's pull request"]),
    ("the second run loaded the dark theme and saved light", s["dark"] is False),
]
bad = [name for name, ok in checks if not ok]
for name, ok in checks:
    print(f"todo-check: {'ok  ' if ok else 'FAIL'} {name}")
sys.exit(1 if bad else 0)
