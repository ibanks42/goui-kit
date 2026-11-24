+++ Welcome to the reader | Reader | 2026-11-12
# Welcome

This is the **reader** example: a list of articles on the left, the open articles as tabs on
the right, each one drawn by the kit's Markdown view.

- Click an article to open it in a tab.
- `ctrl-r` reloads the feed, `ctrl-w` closes the tab, `ctrl-,` opens the settings.
- The settings change the font size, the theme, the order of the list and whether the list
  shows a summary under each title.

> Start it with `READER_FEED=path` to read another feed file or a folder of `.md` files.

+++ Why plain text lasts | Field Notes | 2026-11-11
# Why plain text lasts

A file of plain text written in 1975 opens today in every editor on every machine. Very few
formats can say that.

## What it costs

Almost nothing: no schema, no migration, no vendor. The price is paid in *structure*, which
plain text leaves to convention.

## Conventions that held

1. Markdown for prose.
2. CSV for tables, with all its quirks.
3. JSON for anything a program writes and a person sometimes reads.

| format | born | still readable |
|---|---|---|
| plain text | 1960s | yes |
| RTF | 1987 | mostly |
| WordPerfect 5.1 | 1989 | with effort |

+++ A week with a mechanical keyboard | Desk Setup | 2026-11-10
# A week with a mechanical keyboard

The first day was loud. The second was louder. By the fifth I stopped hearing it.

What changed was not speed but **accuracy**: a key that clicks is a key that was pressed.

```
words per minute   before  after
day 1                  78     71
day 7                  78     82
```

Would I go back? Only to the laptop, and only on trains.

+++ Notes on caching | Systems Weekly | 2026-11-09
# Notes on caching

There are two hard things, and this article is about one of them.

## Keys

A cache is only as good as its key. A key that leaves out an input returns a stale answer;
a key that includes too much never hits.

## Invalidation

```go
func invalidate(c *Cache, key string) {
	delete(c.entries, key)
}
```

> The cheapest invalidation is a key that changes by itself when the input does.

+++ Bread without a recipe | Kitchen Table | 2026-11-08
# Bread without a recipe

Flour, water, salt, yeast. The ratios matter less than the time.

- 500 g flour
- 350 g water
- 10 g salt
- a pinch of yeast, and a night

Fold it four times in the first two hours, then leave it alone. Bake hot, in a closed pot,
for forty minutes.

+++ The quiet hour | Field Notes | 2026-11-07
# The quiet hour

Between six and seven in the morning nothing asks for anything. No messages, no meetings,
no news worth reading yet.

It is the only hour of the day that is *entirely* yours, which is why it is the first one to
be given away.

+++ Release notes 0.9 | Project Log | 2026-11-06
# Release notes 0.9

## New

- A markdown view that draws tables, code and quotes.
- Tabs that can be closed with the middle button.

## Fixed

- The list no longer jumps when an item is removed above the view.
- Dark theme colours for code blocks.

## Known issues

Very long lines in code blocks are cut, not wrapped.

+++ Walking the long way | Field Notes | 2026-11-05
# Walking the long way

The short way to the station is eight minutes along the main road. The long way is fourteen
minutes through the park.

The six minutes cost nothing and buy a morning that starts with trees instead of traffic.
