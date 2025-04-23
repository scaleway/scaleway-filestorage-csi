package scaleway

import (
	"errors"

	"github.com/scaleway/scaleway-sdk-go/scw"
)

// IsNotFoundError returns true if the provided error is a ResourceNotFoundError.
func IsNotFoundError(err error) bool {
	var notFound *scw.ResourceNotFoundError
	return errors.As(err, &notFound)
}

// IsInvalidArgumentsError returns true if the provided error is an InvalidArgumentsError.
func IsInvalidArgumentsError(err error) bool {
	var invalidArguments *scw.InvalidArgumentsError
	return errors.As(err, &invalidArguments)
}
