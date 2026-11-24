package rt

// TableRec stores the row and column dimensions for a RowSource backing a
// virtualized table, used by github.com/ibanks42/goui-kit/gk/table to map row/column indices
// to flat row-source entries and validate updates.
type TableRec struct{ Rows, Cols int }

// Tables maps RowSource handles to their TableRec dimensions. Entries are
// cleaned up automatically in releaseSource when the source is released.
var Tables = map[uint64]*TableRec{}
