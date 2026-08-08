//go:build stress && !race

package transport

// raceEnabled reports whether the test binary was built with the race
// detector. It is referenced only by the stress test, so this file also
// carries the stress tag to keep normal builds free of an unused constant.
const raceEnabled = false
