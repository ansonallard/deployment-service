package utils

func StrPtr(s string) *string {
	return &s
}

func ReturnPathWithTrailingSlash(path string) string {
	const trailingSlash = "/"
	if len(path) == 0 {
		return trailingSlash
	}
	if string(path[len(path)-1]) != trailingSlash {
		return path + trailingSlash
	}
	return path
}

func ToAddress[T any](x T) *T {
	return &x
}
