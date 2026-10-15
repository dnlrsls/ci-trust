package discovery

// Scan reads a bounded snapshot and returns a static intermediate inventory.
// It never executes discovered commands or establishes CI execution.
func Scan(path string, options Options) Result {
	return Analyze(ReadSnapshot(path, options), options)
}
