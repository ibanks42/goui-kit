package rt

// TableRec is a table's shape, by its row source (package table keeps it; a released source's
// goes with it, rt_source.go).
type TableRec struct{ Rows, Cols int }

// Tables holds every table's shape by its row source's handle.
var Tables = map[uint64]*TableRec{}
