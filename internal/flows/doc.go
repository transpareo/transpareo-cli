// Package flows holds the multi-step operations that both the
// command line and the MCP server offer: a spreadsheet import
// from upload to execution, an export from start to download,
// and the event feed as a stream. Each flow is one type with one
// Run method, so the two front ends share one implementation.
package flows
