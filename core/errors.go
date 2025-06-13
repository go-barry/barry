package core

import "errors"

var ErrNotFound = errors.New("barry: not found")

func IsNotFoundError(err error) bool {
	return err != nil && err.Error() == ErrNotFound.Error()
}
