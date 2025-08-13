package common

func Output[T any](to chan T, from <-chan T) {
	go func() {
		for msg := range from {
			to <- msg
		}
	}()
}

func Merge[C any](channels ...<-chan C) <-chan C {
	merged := make(chan C)

	go func() {
		defer close(merged)
		for _, ch := range channels {
			for msg := range ch {
				merged <- msg
			}
		}
	}()

	return merged
}
