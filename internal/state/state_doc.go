// Package state is commando's file-based persistence layer. Instead of an opaque
// database, all mutable state lives in plain, human-readable TOML files the user
// can inspect and edit:
//
//   - memory.toml — remembered variable values (for argument memory)
//   - fills.toml  — remembered completed forms (for recent fills)
//   - usage.toml  — per-entry launch counts and last-used times (for ranking)
//
// Shell command history is NOT stored here; it is read live from the shell's
// history file by the history provider. Ordering decisions live in the rank
// package, not here — this layer only records and returns usage data.
package state
