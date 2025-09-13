package utils

func ToAddress[T any](x T) *T {
	return &x
}
