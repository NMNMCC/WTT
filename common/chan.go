package common

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
