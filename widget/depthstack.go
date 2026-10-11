package widget

// depthStack hands each level of a recursive layout pass its own
// reusable buffer, the walkStack pattern: a level holds its buffer
// across its whole loop while the passes it runs for its children sit
// one level deeper, so no buffer is rewritten while it is still read,
// and a steady frame allocates nothing once the buffers have grown.
// Layout runs on the loop goroutine (docs/threading.md); one stack per
// buffer kind serves the process.
type depthStack[T any] struct {
	bufs  [][]T
	depth int
}

// push descends one level and returns that level's buffer, n zeroed
// elements long. The caller owns it until the matching pop.
func (s *depthStack[T]) push(n int) []T {
	for len(s.bufs) <= s.depth {
		s.bufs = append(s.bufs, nil)
	}
	b := s.bufs[s.depth]
	if cap(b) < n {
		b = make([]T, n)
	}
	b = b[:n]
	clear(b)
	s.bufs[s.depth] = b
	s.depth++
	return b
}

// pop releases the deepest level's buffer.
func (s *depthStack[T]) pop() { s.depth-- }
