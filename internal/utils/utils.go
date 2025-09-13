package utils

import (
	"github.com/oklog/ulid/v2"
)

func ToAddress[T any](x T) *T {
	return &x
}

func GenerateUlid() ulid.ULID {
	return ulid.Make()
}

func GenerateUlidString() string {
	return GenerateUlid().String()
}
