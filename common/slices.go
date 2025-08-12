package common

func SafeGet[T any](s []T, i int) (val T, ok bool) {
	if i < 0 || i >= len(s) {
		var zero T
		return zero, false
	}
	return s[i], true
}
