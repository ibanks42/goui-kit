// reader: a feed reader. The articles of a feed file (or of a folder of .md files) in a list on
// the left, the open articles as tabs on the right, each drawn by the kit's Markdown view; a
// toolbar of icon buttons with tooltips, a settings dialog (font size, theme, order, summaries),
// a notification when a load finishes, and the article fading in when it is opened.
//
//   examples/odin/build.sh reader && bin/reader-odin       a real window
//   READER_FEED=path   a feed file (articles after "+++ title | source | date" lines) or a folder
//                      of .md files; default the bundled sample-feed.md
//   GK_SCRIPT=file     offscreen under a gk-meter script (the diag library; gates/reader.sh)
package reader

import "core:fmt"
import "core:os"
import "core:path/filepath"
import "core:slice"
import "core:strings"
import "core:thread"
import "core:time"
import gk "gpuikit:gk"

SAMPLE :: #load("sample-feed.md", string)

Article :: struct {
	title, source, date, body: string,
	read, starred:             bool,
}

Order :: enum {
	Newest,
	Oldest,
	Title,
}

App :: struct {
	feed:         string, // READER_FEED or ""
	articles:     []Article,
	shown:        [dynamic]int, // the articles the list shows, in order
	tabs:         [dynamic]int, // open articles
	active:       int, // the tab shown, -1 none
	loading:      bool,
	opened:       int, // how many times an article was opened, for the fade-in's key
	// settings
	font_size:    f32,
	dark:         bool,
	order:        Order,
	summaries:    bool,
	// kit state
	list:         gk.List_State,
	search:       gk.Input_State,
	font:         gk.Slider_State,
	theme_select: gk.Select_State,
	order_select: gk.Select_State,
}

// --- loading, on a thread -------------------------------------------------------------------------

// Loaded is what the loading thread posts: the message is copied, articles and all.
Loaded :: struct {
	articles: []Article,
	took:     time.Duration,
	err:      string,
}

load_feed :: proc(path: string) {
	context.allocator = context.temp_allocator // the thread's own; gk.post copies what it sends
	defer free_all(context.temp_allocator)
	start := time.now()
	out: [dynamic]Article
	err := ""
	if path == "" {
		parse_feed(SAMPLE, &out)
	} else if os.is_dir(path) {
		pattern, _ := filepath.join({path, "*.md"}, context.temp_allocator)
		files, _ := filepath.glob(pattern)
		slice.sort(files)
		for f in files {
			data, rerr := os.read_entire_file(f, context.allocator)
			if rerr != nil {continue}
			text := string(data)
			title := filepath.stem(f)
			if strings.has_prefix(text, "# ") {
				title = text[2:strings.index_byte(text, '\n')] if strings.contains(text, "\n") else text[2:]
			}
			append(&out, Article{title = title, source = filepath.base(path), date = "", body = text})
		}
	} else if data, rerr := os.read_entire_file(path, context.allocator); rerr == nil {
		parse_feed(string(data), &out)
	} else {
		err = fmt.aprintf("cannot read %s", path)
	}
	gk.post(loaded, Loaded{out[:], time.since(start), err})
}

// parse_feed reads "+++ title | source | date" lines, each followed by the article's Markdown.
parse_feed :: proc(text: string, out: ^[dynamic]Article) {
	rest := text
	for len(rest) > 0 {
		if !strings.has_prefix(rest, "+++ ") {
			nl := strings.index(rest, "\n+++ ")
			if nl < 0 {break}
			rest = rest[nl + 1:]
		}
		eol := strings.index_byte(rest, '\n')
		if eol < 0 {eol = len(rest)}
		head := strings.split(rest[4:eol], "|", context.temp_allocator)
		rest = rest[min(eol + 1, len(rest)):]
		next := strings.index(rest, "\n+++ ")
		body := rest if next < 0 else rest[:next + 1]
		rest = "" if next < 0 else rest[next + 1:]
		a := Article{body = strings.trim_space(body)}
		if len(head) > 0 {a.title = strings.trim_space(head[0])}
		if len(head) > 1 {a.source = strings.trim_space(head[1])}
		if len(head) > 2 {a.date = strings.trim_space(head[2])}
		append(out, a)
	}
}

start_load :: proc(a: ^App, cx: ^gk.Cx) {
	if a.loading {return}
	a.loading = true
	thread.create_and_start_with_poly_data(a.feed, load_feed, self_cleanup = true)
}

loaded :: proc(a: ^App, cx: ^gk.Cx, m: Loaded) {
	a.loading = false
	if m.err != "" {
		gk.push_notification(cx, m.err)
		gk.refresh(cx)
		return
	}
	delete(a.articles)
	a.articles = m.articles // the message is the procedure's: keep its articles
	clear(&a.tabs)
	a.active = -1
	resort(a, cx) // and refresh
	gk.push_notification(cx, fmt.tprintf("Loaded %d articles in %.0f ms", len(a.articles), time.duration_milliseconds(m.took)))
}

// resort: the list's articles, filtered by the search and in the chosen order; then refresh.
resort :: proc(a: ^App, cx: ^gk.Cx) {
	clear(&a.shown)
	needle := strings.to_lower(string(gk.value(a.search, cx)), context.temp_allocator)
	for art, i in a.articles {
		if needle == "" || strings.contains(strings.to_lower(art.title, context.temp_allocator), needle) {
			append(&a.shown, i)
		}
	}
	context.user_ptr = a
	switch a.order {
	case .Newest:
		slice.stable_sort_by(a.shown[:], proc(x, y: int) -> bool {
			a := (^App)(context.user_ptr)
			return a.articles[x].date > a.articles[y].date
		})
	case .Oldest:
		slice.stable_sort_by(a.shown[:], proc(x, y: int) -> bool {
			a := (^App)(context.user_ptr)
			return a.articles[x].date < a.articles[y].date
		})
	case .Title:
		slice.stable_sort_by(a.shown[:], proc(x, y: int) -> bool {
			a := (^App)(context.user_ptr)
			return a.articles[x].title < a.articles[y].title
		})
	}
	select_active(a, cx)
}

// select_active selects the open article's row in the list, if the list shows it.
select_active :: proc(a: ^App, cx: ^gk.Cx, scroll := false) {
	row := -1
	if a.active >= 0 {
		row, _ = slice.linear_search(a.shown[:], a.tabs[a.active])
	}
	gk.select_row(cx, a.list, row, scroll = scroll)
	gk.refresh(cx)
}

shown_count :: proc(a: ^App) -> int {return len(a.shown)}

// --- init -----------------------------------------------------------------------------------------

THEMES := [?]gk.Search_Item{{value = "light", title = "Light"}, {value = "dark", title = "Dark"}}
ORDERS := [?]gk.Search_Item{{value = "newest", title = "Newest first"}, {value = "oldest", title = "Oldest first"}, {value = "title", title = "By title"}}

init :: proc(a: ^App, cx: ^gk.Cx) {
	a.active = -1
	a.font_size = 15
	a.summaries = true

	gk.shortcut(cx, "ctrl-r", gk.on(reload))
	gk.shortcut(cx, "ctrl-w", gk.on(close_active))
	gk.shortcut(cx, "ctrl-,", gk.on(open_settings))
	gk.shortcut(cx, "alt-down", gk.on(next_article))
	gk.shortcut(cx, "alt-up", gk.on(prev_article))

	a.search = gk.input_state(cx)->placeholder("Search titles")->build()
	gk.on_input_event(a.search, cx, gk.on(search_changed))
	a.list = gk.list_state(cx, gk.list_source(cx, a, shown_count, article_row))->build()
	gk.on_list_event(a.list, cx, gk.on(list_event))

	a.font = gk.slider_state(cx)->min(12)->max(24)->step(1)->default_value(a.font_size)->build()
	gk.on_slider_event(a.font, cx, gk.on(font_changed))
	a.theme_select = gk.select_state(cx, gk.search_source(cx, a, THEMES[:]), nil)->build()
	gk.set_selected_value(a.theme_select, cx, "light")
	gk.on_select_event(a.theme_select, cx, gk.on(theme_selected))
	a.order_select = gk.select_state(cx, gk.search_source(cx, a, ORDERS[:]), nil)->build()
	gk.set_selected_value(a.order_select, cx, "newest")
	gk.on_select_event(a.order_select, cx, gk.on(order_selected))

	start_load(a, cx)
}

// --- the views ------------------------------------------------------------------------------------

root_view :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	return gk.v_flex(.Size_Full)->bg(gk.theme(.Background))->text_color(gk.theme(.Foreground))->
		child(toolbar(a))->
		child(gk.div(.Flex_1, .W_Full, .Min_H_0)->child(gk.h_resizable("split")->
			child(gk.resizable_panel()->size(300)->child(sidebar(a)))->
			child(gk.resizable_panel()->child(reading_pane(a, cx)))))
}

toolbar :: proc(a: ^App) -> gk.Element {
	starred := a.active >= 0 && a.articles[a.tabs[a.active]].starred
	return gk.h_flex(.Gap_1, .Items_Center, .P_2, .Border_B_1)->border_color(gk.theme(.Border))->
		child(gk.button("reload")->ghost()->icon(.Refresh_Cw)->tooltip("Reload the feed")->loading(a.loading)->on_click(gk.on(reload)))->
		child(gk.button("prev")->ghost()->icon(.Arrow_Up)->tooltip("Previous article")->on_click(gk.on(prev_article)))->
		child(gk.button("next")->ghost()->icon(.Arrow_Down)->tooltip("Next article")->on_click(gk.on(next_article)))->
		child(gk.button("star")->ghost()->icon(.Star_Fill if starred else .Star)->tooltip("Star")->disabled(a.active < 0)->on_click(gk.on(toggle_star)))->
		child(gk.button("close")->ghost()->icon(.Close)->tooltip("Close the tab")->disabled(a.active < 0)->on_click(gk.on(close_active)))->
		child(gk.div(.Flex_1))->
		child(gk.button("theme")->ghost()->icon(.Sun if a.dark else .Moon)->tooltip("Light theme" if a.dark else "Dark theme")->on_click(gk.on(toggle_theme)))->
		child(gk.button("settings")->ghost()->icon(.Settings)->tooltip("Settings")->on_click(gk.on(open_settings)))
}

sidebar :: proc(a: ^App) -> gk.Element {
	unread := 0
	for art in a.articles {
		unread += int(!art.read)
	}
	body: gk.Element = gk.list(a.list)
	if a.loading && len(a.articles) == 0 {
		body = gk.v_flex(.Size_Full, .Items_Center, .Justify_Center)->child(gk.label("Loading…"))
	}
	return gk.v_flex(.Size_Full, .Gap_2, .P_2)->bg(gk.theme(.Sidebar))->
		child(gk.input(a.search))->
		child(gk.label(fmt.tprintf("%d articles, %d unread", len(a.shown), unread), .Text_Sm)->text_color(gk.theme(.Muted_Foreground)))->
		child(gk.div(.Flex_1, .Min_H_0)->child(body))
}

article_row :: proc(a: ^App, cx: ^gk.Cx, at: gk.Row_At) -> gk.List_Item {
	i := a.shown[at.row]
	art := a.articles[i]
	title := gk.label(art.title, .Truncate)
	if !art.read {
		title = title->style(.Font_Semibold)
	}
	col := gk.v_flex(.Flex_1, .Min_W_0)->
		child(title)->
		child(gk.label(fmt.tprintf("%s · %s", art.source, art.date), .Text_Xs)->text_color(gk.theme(.Muted_Foreground)))
	if a.summaries {
		col = col->child(gk.label(summary(art.body), .Text_Xs, .Truncate)->text_color(gk.theme(.Muted_Foreground)))
	}
	row := gk.h_flex(.W_Full, .Gap_2, .Items_Center)->child(col)
	if art.starred {
		row = row->child(gk.icon(.Star_Fill)->text_color(gk.theme(.Warning)))
	}
	return gk.list_item(gk.id("article", i))->child(row)
}

// summary: the first line of the article that is not a heading.
summary :: proc(body: string) -> string {
	rest := body
	for line in strings.split_lines_iterator(&rest) {
		l := strings.trim_space(line)
		if l != "" && !strings.has_prefix(l, "#") {
			return l
		}
	}
	return ""
}

reading_pane :: proc(a: ^App, cx: ^gk.Cx) -> gk.Element {
	if a.active < 0 {
		return gk.v_flex(.Size_Full, .Items_Center, .Justify_Center)->
			child(gk.label("Open an article from the list")->text_color(gk.theme(.Muted_Foreground)))
	}
	bar := gk.tab_bar("tabs")->selected_index(a.active)->on_click(gk.on(tab_clicked))
	for t, n in a.tabs {
		bar = bar->child(gk.tab()->label(a.articles[t].title)->
			suffix(gk.button(gk.id("close-tab", n))->ghost()->xsmall()->icon(.Close)->on_click(gk.on(close_tab_at, n))))
	}
	art := a.articles[a.tabs[a.active]]
	page := gk.text_view_markdown(gk.id("article", a.tabs[a.active]), art.body)->scrollable(true)->text_size(gk.px(a.font_size))
	fade := gk.effect_transition(200 * time.Millisecond)->fade(0, 1)->slide_y(12, 0)
	return gk.v_flex(.Size_Full)->
		child(bar)->
		child(gk.div(.Flex_1, .Min_H_0, .P_4)->child(gk.effect(page, fade, fmt.tprintf("open-%d", a.opened))))
}

// --- the settings dialog --------------------------------------------------------------------------

settings_dialog :: proc(a: ^App, cx: ^gk.Cx) -> gk.Dialog {
	return gk.dialog()->title(gk.label("Settings"))->
		child(gk.v_flex(.Gap_4)->
			child(gk.v_flex(.Gap_1)->
				child(gk.label(fmt.tprintf("Font size: %.0f px", a.font_size)))->
				child(gk.slider(a.font)))->
			child(gk.h_flex(.Gap_2, .Items_Center)->child(gk.label("Theme")->w(gk.px(120)))->child(gk.select(a.theme_select)))->
			child(gk.h_flex(.Gap_2, .Items_Center)->child(gk.label("Order")->w(gk.px(120)))->child(gk.select(a.order_select)))->
			child(gk.switch_("summaries")->label("Show a summary under each title")->checked(a.summaries)->on_click(gk.on(summaries_toggled))))->
		footer(gk.dialog_footer()->child(gk.dialog_close()->trigger(gk.button("done")->primary()->label("Done"))))
}

open_settings :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_dialog(cx, gk.built(cx, a, settings_dialog))
}

font_changed :: proc(a: ^App, cx: ^gk.Cx, e: gk.Slider_Event) {
	a.font_size = gk.slider_number(e)
	gk.refresh(cx)
}

theme_selected :: proc(a: ^App, cx: ^gk.Cx, e: gk.Select_Event) {
	if v, ok := e.v0.?; ok {
		set_dark(a, cx, string(v) == "dark")
	}
}

order_selected :: proc(a: ^App, cx: ^gk.Cx, e: gk.Select_Event) {
	if v, ok := e.v0.?; ok {
		switch string(v) {
		case "newest": a.order = .Newest
		case "oldest": a.order = .Oldest
		case "title": a.order = .Title
		}
		resort(a, cx) // and refresh
	}
}

summaries_toggled :: proc(a: ^App, cx: ^gk.Cx, on: bool) {
	a.summaries = on
	gk.refresh(cx)
}

set_dark :: proc(a: ^App, cx: ^gk.Cx, dark: bool) {
	a.dark = dark
	gk.theme_change(cx, gk.Theme_Mode.Dark if dark else gk.Theme_Mode.Light)
	gk.set_selected_value(a.theme_select, cx, "dark" if dark else "light")
	gk.refresh(cx)
}

toggle_theme :: proc(a: ^App, cx: ^gk.Cx) {
	set_dark(a, cx, !a.dark)
}

// --- what the app does ----------------------------------------------------------------------------

reload :: proc(a: ^App, cx: ^gk.Cx) {
	start_load(a, cx)
	gk.refresh(cx)
}

search_changed :: proc(a: ^App, cx: ^gk.Cx, e: gk.Input_Event) {
	if _, ok := e.(gk.Input_Event_Change); ok {
		resort(a, cx) // and refresh
	}
}

list_event :: proc(a: ^App, cx: ^gk.Cx, e: gk.List_Event) {
	#partial switch v in e {
	case gk.List_Event_Confirm:
		open_article(a, cx, a.shown[v.row])
	case gk.List_Event_Select:
		open_article(a, cx, a.shown[v.row])
	}
}

open_article :: proc(a: ^App, cx: ^gk.Cx, i: int) {
	n, found := slice.linear_search(a.tabs[:], i)
	if !found {
		append(&a.tabs, i)
		n = len(a.tabs) - 1
	}
	if n != a.active {
		a.active = n
		a.opened += 1
	}
	a.articles[i].read = true
	select_active(a, cx, scroll = true)
}

tab_clicked :: proc(a: ^App, cx: ^gk.Cx, n: int) {
	open_article(a, cx, a.tabs[n])
}

close_tab_at :: proc(a: ^App, cx: ^gk.Cx, n: int) {
	gk.stop_propagation(cx) // the tab itself is not clicked
	ordered_remove(&a.tabs, n)
	if n < a.active || a.active == len(a.tabs) {
		a.active -= 1
	}
	a.opened += 1
	select_active(a, cx)
}

close_active :: proc(a: ^App, cx: ^gk.Cx) {
	if a.active >= 0 {
		close_tab_at(a, cx, a.active)
	}
}

step_article :: proc(a: ^App, cx: ^gk.Cx, by: int) {
	if len(a.shown) == 0 {return}
	row := -1
	if a.active >= 0 {
		row, _ = slice.linear_search(a.shown[:], a.tabs[a.active])
	}
	open_article(a, cx, a.shown[clamp(row + by, 0, len(a.shown) - 1)])
}

next_article :: proc(a: ^App, cx: ^gk.Cx) {step_article(a, cx, 1)}
prev_article :: proc(a: ^App, cx: ^gk.Cx) {step_article(a, cx, -1)}

toggle_star :: proc(a: ^App, cx: ^gk.Cx) {
	if a.active >= 0 {
		art := &a.articles[a.tabs[a.active]]
		art.starred = !art.starred
		gk.refresh(cx)
	}
}

// --- start ----------------------------------------------------------------------------------------

start :: proc(a: ^App, cx: ^gk.Cx) {
	gk.open_window(cx, "Reader", {1000, 660}, gk.view(a, init, root_view))
}

main :: proc() {
	app: App
	app.feed = os.lookup_env("READER_FEED", context.allocator) or_else ""
	gk.run(&app, start)
}
