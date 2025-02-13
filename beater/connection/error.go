package connection

import "errors"

var (
	errUnknownInterface = errors.New("interface invalid")
	errInvalidMulticast = errors.New("multicast address invalid")
	errMissingArguments = errors.New("no interfaces or multicasts")
)
