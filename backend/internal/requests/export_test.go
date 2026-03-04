package requests

// SetLaunch overrides the launch function for synchronous testing.
// Call this with func(f func()) { f() } to run the import pipeline
// synchronously so tests can make deterministic assertions.
func SetLaunch(h *Handler, f func(func())) {
	h.launch = f
}

// SetDeleteFromQBit overrides deleteFromQBit for testing.
func SetDeleteFromQBit(h *Handler, enabled bool) {
	h.deleteFromQBit = enabled
}
