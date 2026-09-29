package judging

// rand is a small, explicitly specified pseudo-random generator.
//
// The bootstrap has to be reproducible: a results endpoint whose confidence
// intervals move on every refresh cannot be audited, and an archived result set
// cannot be recomputed. math/rand would work today, but its stream is not
// contractually stable across Go releases, so a future toolchain bump would
// silently change published numbers. This is splitmix64, which is fully
// specified by its own constants and will produce the same stream forever.
type rand struct{ state uint64 }

func newRand(seed int64) *rand { return &rand{state: uint64(seed)} }

func (r *rand) next() uint64 {
	r.state += 0x9E3779B97F4A7C15
	z := r.state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// intn returns a value in [0, n). The rejection loop removes the modulo bias
// that would otherwise slightly favour low judge indices, which matters when
// the sample size is a handful of judges.
func (r *rand) intn(n int) int {
	if n <= 0 {
		return 0
	}
	bound := uint64(n)
	limit := ^uint64(0) - (^uint64(0) % bound) - 1
	for {
		value := r.next()
		if value <= limit {
			return int(value % bound)
		}
	}
}
