// reader: a feed reader. The articles of a feed file (or of a
// folder of .md files) in a list on the left, the open articles as tabs on the right, each drawn
// by the kit's Markdown view; a toolbar of icon buttons with tooltips, a settings dialog (font
// size, theme, order, summaries), a notification when a load finishes, and the article fading in
// when it is opened.
//
//	reader-go                  a real window
//	READER_FEED=path           a feed file (articles after "+++ title | source | date" lines) or a
//	                           folder of .md files; default the bundled sample-feed.md
//	GK_SCRIPT=file reader-go   offscreen under a gk-meter script (gates/reader.sh)
package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gpuikit.dev/gk"
	"gpuikit.dev/gk/s"
	"gpuikit.dev/gk/theme"
)

//go:embed sample-feed.md
var sample string

type Article struct {
	Title, Source, Date, Body string
	Read, Starred             bool
}

type Order int

const (
	Newest Order = iota
	Oldest
	ByTitle
)

type App struct {
	feed     string    // READER_FEED or ""
	articles []Article // owned by the UI thread
	shown    []int     // the articles the list shows, in order
	tabs     []int     // open articles
	active   int       // the tab shown, -1 none
	loading  bool
	opened   int // how many times an article was opened, for the fade-in's key
	// settings
	fontSize  float32
	dark      bool
	order     Order
	summaries bool
	// kit state
	list                     gk.ListState
	search                   gk.InputState
	font                     gk.SliderState
	themeSelect, orderSelect gk.SelectState
	settings                 gk.Built[gk.DialogEl]
}

// --- loading, on a goroutine ------------------------------------------------------------------

type loaded struct {
	articles []Article
	took     time.Duration
	err      error
}

func loadFeed(path string) loaded {
	start := time.Now()
	var out []Article
	switch info, err := os.Stat(path); {
	case path == "":
		out = parseFeed(sample)
	case err == nil && info.IsDir():
		files, _ := filepath.Glob(filepath.Join(path, "*.md"))
		slices.Sort(files)
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			text := string(data)
			title := strings.TrimSuffix(filepath.Base(f), ".md")
			if rest, ok := strings.CutPrefix(text, "# "); ok {
				title, _, _ = strings.Cut(rest, "\n")
			}
			out = append(out, Article{Title: title, Source: filepath.Base(path), Body: text})
		}
	default:
		data, err := os.ReadFile(path)
		if err != nil {
			return loaded{err: fmt.Errorf("cannot read %s", path)}
		}
		out = parseFeed(string(data))
	}
	return loaded{out, time.Since(start), nil}
}

// parseFeed reads "+++ title | source | date" lines, each followed by the article's Markdown.
func parseFeed(text string) []Article {
	var out []Article
	rest := text
	for rest != "" {
		if !strings.HasPrefix(rest, "+++ ") {
			nl := strings.Index(rest, "\n+++ ")
			if nl < 0 {
				break
			}
			rest = rest[nl+1:]
		}
		head, after, _ := strings.Cut(rest[4:], "\n")
		body := after
		rest = ""
		if next := strings.Index(after, "\n+++ "); next >= 0 {
			body, rest = after[:next+1], after[next+1:]
		}
		a := Article{Body: strings.TrimSpace(body)}
		parts := strings.Split(head, "|")
		for i, p := range []*string{&a.Title, &a.Source, &a.Date} {
			if i < len(parts) {
				*p = strings.TrimSpace(parts[i])
			}
		}
		out = append(out, a)
	}
	return out
}

func (a *App) startLoad(cx *gk.Cx) {
	if a.loading {
		return
	}
	a.loading = true
	feed := a.feed
	gk.Go(cx, func() loaded { return loadFeed(feed) }, a.loaded)
}

func (a *App) loaded(cx *gk.Cx, m loaded) {
	a.loading = false
	if m.err != nil {
		gk.PushNotification(cx, m.err.Error())
		gk.Refresh(cx)
		return
	}
	a.articles, a.tabs, a.active = m.articles, nil, -1
	a.resort(cx) // and refresh
	gk.PushNotification(cx, fmt.Sprintf("Loaded %d articles in %.0f ms", len(a.articles), float64(m.took)/float64(time.Millisecond)))
}

// resort: the list's articles, filtered by the search and in the chosen order; then refresh.
func (a *App) resort(cx *gk.Cx) {
	a.shown = a.shown[:0]
	needle := strings.ToLower(a.search.Value(cx))
	for i, art := range a.articles {
		if needle == "" || strings.Contains(strings.ToLower(art.Title), needle) {
			a.shown = append(a.shown, i)
		}
	}
	key := func(i int) string { return a.articles[i].Date }
	if a.order == ByTitle {
		key = func(i int) string { return a.articles[i].Title }
	}
	slices.SortStableFunc(a.shown, func(x, y int) int {
		if a.order == Newest {
			x, y = y, x
		}
		return strings.Compare(key(x), key(y))
	})
	a.selectActive(cx)
}

// selectActive selects the open article's row in the list, if the list shows it.
func (a *App) selectActive(cx *gk.Cx, opts ...gk.SelectOption) {
	row := -1
	if a.active >= 0 {
		row = slices.Index(a.shown, a.tabs[a.active])
	}
	gk.SelectRow(cx, a.list, row, opts...)
	gk.Refresh(cx)
}

// --- init -------------------------------------------------------------------------------------

var (
	themes = []gk.SearchItem{{Value: "light", Title: "Light"}, {Value: "dark", Title: "Dark"}}
	orders = []gk.SearchItem{{Value: "newest", Title: "Newest first"}, {Value: "oldest", Title: "Oldest first"}, {Value: "title", Title: "By title"}}
)

func (a *App) init(cx *gk.Cx) {
	a.active, a.fontSize, a.summaries = -1, 15, true

	gk.Shortcut(cx, "ctrl-r", a.reload)
	gk.Shortcut(cx, "ctrl-w", a.closeActive)
	gk.Shortcut(cx, "ctrl-,", a.openSettings)
	gk.Shortcut(cx, "alt-down", a.nextArticle)
	gk.Shortcut(cx, "alt-up", a.prevArticle)

	a.search = gk.NewInputState(cx).Placeholder("Search titles").Build()
	a.search.OnInputEvent(cx, func(cx *gk.Cx, e gk.InputEvent) {
		if _, ok := e.(gk.InputEventChange); ok {
			a.resort(cx)
		}
	})
	a.list = gk.NewListState(cx, gk.NewListSource(cx, func() int { return len(a.shown) }, a.articleRow)).Build()
	a.list.OnListEvent(cx, a.listEvent)

	a.font = gk.NewSliderState(cx).Min(12).Max(24).Step(1).DefaultValue(a.fontSize).Build()
	a.font.OnSliderEvent(cx, func(cx *gk.Cx, e gk.SliderEvent) {
		a.fontSize = gk.SliderNumber(e)
		gk.Refresh(cx)
	})
	a.themeSelect = gk.NewSelectState(cx, gk.NewSearchSource(cx, themes), gk.NoRow).Build()
	a.themeSelect.SetSelectedValue(cx, "light")
	a.themeSelect.OnSelectEvent(cx, func(cx *gk.Cx, e gk.SelectEvent) {
		if v, ok := e.(gk.SelectEventConfirm).Value.Get(); ok {
			a.setDark(cx, v == "dark")
		}
	})
	a.orderSelect = gk.NewSelectState(cx, gk.NewSearchSource(cx, orders), gk.NoRow).Build()
	a.orderSelect.SetSelectedValue(cx, "newest")
	a.orderSelect.OnSelectEvent(cx, func(cx *gk.Cx, e gk.SelectEvent) {
		if v, ok := e.(gk.SelectEventConfirm).Value.Get(); ok {
			a.order = map[string]Order{"newest": Newest, "oldest": Oldest, "title": ByTitle}[v]
			a.resort(cx) // and refresh
		}
	})
	a.settings = gk.NewBuilt(cx, a.settingsView)

	a.startLoad(cx)
}

// --- the views --------------------------------------------------------------------------------

func (a *App) view(cx *gk.Cx) gk.Element {
	return gk.VFlex(s.SizeFull).Bg(theme.Background).TextColor(theme.Foreground).
		Child(a.toolbar()).
		Child(gk.Div(s.Flex1, s.WFull, s.MinH0).Child(gk.HResizable("split").
			Child(gk.ResizablePanel().Size(300).Child(a.sidebar())).
			Child(gk.ResizablePanel().Child(a.readingPane()))))
}

func (a *App) toolbar() gk.Element {
	starred := a.active >= 0 && a.articles[a.tabs[a.active]].Starred
	star, themeIcon, themeTip := gk.IconStar, gk.IconMoon, "Dark theme"
	if starred {
		star = gk.IconStarFill
	}
	if a.dark {
		themeIcon, themeTip = gk.IconSun, "Light theme"
	}
	return gk.HFlex(s.Gap1, s.ItemsCenter, s.P2, s.BorderB1).BorderColor(theme.Border).
		Child(gk.Button("reload").Ghost().Icon(gk.IconRefreshCw).Tooltip("Reload the feed").Loading(a.loading).OnClick(a.reload)).
		Child(gk.Button("prev").Ghost().Icon(gk.IconArrowUp).Tooltip("Previous article").OnClick(a.prevArticle)).
		Child(gk.Button("next").Ghost().Icon(gk.IconArrowDown).Tooltip("Next article").OnClick(a.nextArticle)).
		Child(gk.Button("star").Ghost().Icon(star).Tooltip("Star").Disabled(a.active < 0).OnClick(a.toggleStar)).
		Child(gk.Button("close").Ghost().Icon(gk.IconClose).Tooltip("Close the tab").Disabled(a.active < 0).OnClick(a.closeActive)).
		Child(gk.Div(s.Flex1)).
		Child(gk.Button("theme").Ghost().Icon(themeIcon).Tooltip(themeTip).OnClick(func(cx *gk.Cx, _ gk.ClickEvent) { a.setDark(cx, !a.dark) })).
		Child(gk.Button("settings").Ghost().Icon(gk.IconSettings).Tooltip("Settings").OnClick(a.openSettings))
}

func (a *App) sidebar() gk.Element {
	unread := 0
	for _, art := range a.articles {
		if !art.Read {
			unread++
		}
	}
	var body gk.Element = gk.List(a.list)
	if a.loading && len(a.articles) == 0 {
		body = gk.VFlex(s.SizeFull, s.ItemsCenter, s.JustifyCenter).Child(gk.Label("Loading…"))
	}
	return gk.VFlex(s.SizeFull, s.Gap2, s.P2).Bg(theme.Sidebar).
		Child(gk.Input(a.search)).
		Child(gk.Label(fmt.Sprintf("%d articles, %d unread", len(a.shown), unread), s.TextSm).TextColor(theme.MutedForeground)).
		Child(gk.Div(s.Flex1, s.MinH0).Child(body))
}

func (a *App) articleRow(cx *gk.Cx, at gk.RowAt) gk.ListItemEl {
	i := a.shown[at.Row]
	art := a.articles[i]
	title := gk.Label(art.Title, s.Truncate)
	if !art.Read {
		title = title.Style(s.FontSemibold)
	}
	col := gk.VFlex(s.Flex1, s.MinW0).
		Child(title).
		Child(gk.Label(art.Source+" · "+art.Date, s.TextXs).TextColor(theme.MutedForeground))
	if a.summaries {
		col = col.Child(gk.Label(summary(art.Body), s.TextXs, s.Truncate).TextColor(theme.MutedForeground))
	}
	row := gk.HFlex(s.WFull, s.Gap2, s.ItemsCenter).Child(col)
	if art.Starred {
		row = row.Child(gk.Icon(gk.IconStarFill).TextColor(theme.Warning))
	}
	return gk.ListItem(gk.Key("article", i)).Child(row)
}

// summary is the first line of the article that is not a heading.
func summary(body string) string {
	for line := range strings.Lines(body) {
		if l := strings.TrimSpace(line); l != "" && !strings.HasPrefix(l, "#") {
			return l
		}
	}
	return ""
}

func (a *App) readingPane() gk.Element {
	if a.active < 0 {
		return gk.VFlex(s.SizeFull, s.ItemsCenter, s.JustifyCenter).
			Child(gk.Label("Open an article from the list").TextColor(theme.MutedForeground))
	}
	bar := gk.TabBar("tabs").SelectedIndex(a.active).OnClick(func(cx *gk.Cx, n int) { a.open(cx, a.tabs[n]) })
	for n, t := range a.tabs {
		bar = bar.Child(gk.Tab().Label(a.articles[t].Title).
			Suffix(gk.Button(gk.Key("close-tab", n)).Ghost().Xsmall().Icon(gk.IconClose).OnClick(func(cx *gk.Cx, _ gk.ClickEvent) {
				gk.StopPropagation(cx) // the tab itself is not clicked
				a.closeTab(cx, n)
			})))
	}
	i := a.tabs[a.active]
	page := gk.TextViewMarkdown(gk.Key("article", i), a.articles[i].Body).Scrollable(true).TextSize(gk.Px(a.fontSize))
	fade := gk.NewEffectTransition(200*time.Millisecond).Fade(0, 1).SlideY(12, 0)
	return gk.VFlex(s.SizeFull).
		Child(bar).
		Child(gk.Div(s.Flex1, s.MinH0, s.P4).Child(gk.Effect(page, fade, fmt.Sprintf("open-%d", a.opened))))
}

// --- the settings dialog ----------------------------------------------------------------------

func (a *App) settingsView(cx *gk.Cx) gk.DialogEl {
	row := func(label string, sel gk.SelectState) gk.Element {
		return gk.HFlex(s.Gap2, s.ItemsCenter).Child(gk.Label(label).W(gk.Px(120))).Child(gk.Select(sel))
	}
	return gk.Dialog().Title(gk.Label("Settings")).
		Child(gk.VFlex(s.Gap4).
			Child(gk.VFlex(s.Gap1).
				Child(gk.Label(fmt.Sprintf("Font size: %.0f px", a.fontSize))).
				Child(gk.Slider(a.font))).
			Child(row("Theme", a.themeSelect)).
			Child(row("Order", a.orderSelect)).
			Child(gk.Switch("summaries").Label("Show a summary under each title").Checked(a.summaries).
				OnClick(func(cx *gk.Cx, on bool) { a.summaries = on; gk.Refresh(cx) }))).
		Footer(gk.DialogFooter().Child(gk.DialogClose().Trigger(gk.Button("done").Primary().Label("Done"))))
}

func (a *App) openSettings(cx *gk.Cx, _ gk.ClickEvent) { gk.OpenDialog(cx, a.settings) }

func (a *App) setDark(cx *gk.Cx, dark bool) {
	a.dark = dark
	mode, value := gk.ThemeModeLight, "light"
	if dark {
		mode, value = gk.ThemeModeDark, "dark"
	}
	gk.ThemeChange(cx, mode)
	a.themeSelect.SetSelectedValue(cx, value)
	gk.Refresh(cx)
}

// --- what the app does ------------------------------------------------------------------------

func (a *App) reload(cx *gk.Cx, _ gk.ClickEvent) {
	a.startLoad(cx)
	gk.Refresh(cx)
}

func (a *App) listEvent(cx *gk.Cx, e gk.ListEvent) {
	switch e := e.(type) {
	case gk.ListEventConfirm:
		a.open(cx, a.shown[e.Row])
	case gk.ListEventSelect:
		a.open(cx, a.shown[e.Row])
	}
}

func (a *App) open(cx *gk.Cx, i int) {
	n := slices.Index(a.tabs, i)
	if n < 0 {
		a.tabs = append(a.tabs, i)
		n = len(a.tabs) - 1
	}
	if n != a.active {
		a.active = n
		a.opened++
	}
	a.articles[i].Read = true
	a.selectActive(cx, gk.ScrollIntoView)
}

func (a *App) closeTab(cx *gk.Cx, n int) {
	a.tabs = slices.Delete(a.tabs, n, n+1)
	if n < a.active || a.active == len(a.tabs) {
		a.active--
	}
	a.opened++
	a.selectActive(cx)
}

func (a *App) closeActive(cx *gk.Cx, _ gk.ClickEvent) {
	if a.active >= 0 {
		a.closeTab(cx, a.active)
	}
}

func (a *App) step(cx *gk.Cx, by int) {
	if len(a.shown) == 0 {
		return
	}
	row := -1
	if a.active >= 0 {
		row = slices.Index(a.shown, a.tabs[a.active])
	}
	a.open(cx, a.shown[max(0, min(row+by, len(a.shown)-1))])
}

func (a *App) nextArticle(cx *gk.Cx, _ gk.ClickEvent) { a.step(cx, 1) }
func (a *App) prevArticle(cx *gk.Cx, _ gk.ClickEvent) { a.step(cx, -1) }

func (a *App) toggleStar(cx *gk.Cx, _ gk.ClickEvent) {
	if a.active >= 0 {
		art := &a.articles[a.tabs[a.active]]
		art.Starred = !art.Starred
		gk.Refresh(cx)
	}
}

func main() {
	a := &App{feed: os.Getenv("READER_FEED")}
	gk.Run(func(cx *gk.Cx) {
		gk.OpenWindow(cx, "Reader", 1000, 660, gk.NewView(a.init, a.view))
	})
}
