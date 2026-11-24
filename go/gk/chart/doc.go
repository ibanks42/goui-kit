// Package chart provides data-driven charts and composable canvas plots backed
// by gpui-kit.
//
// Wrap any Go slice of structs (or numeric scalars) with [NewData] to expose
// exported fields as named columns validated on the Go side before encoding
// onto the view tape. Use [Plot] and [PlotWith] together with parts such as
// [NewPlotLine], [NewPlotArea], [NewPlotQuad], grids, and axes to build custom
// interactive plots.
package chart
