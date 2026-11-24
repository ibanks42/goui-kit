// Package table provides virtualized data tables, scrollable lists, searchable
// lists, selects, comboboxes, command palettes, and hierarchical trees backed
// by [gk.RowSource].
//
// Use [NewSource] with [TableColumn] definitions and a cell render callback to
// build tables that materialize only visible rows, [NewListSource] for
// virtualized lists synced on [gk.Refresh], and [NewTreeSource] with
// [SetTreeItems] for expandable trees. Delegate interactions (sorting, column
// moves, selection, search, and load-more triggers) arrive via [Events] and
// [ListEvents].
package table
