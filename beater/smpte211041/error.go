package smpte211041

import "errors"

var (
	errHeaderSizeInsufficient          = errors.New("2110-41 header size insufficient")
	errHeaderSizeInsufficientForOffset = errors.New("2110-41 header size insufficient for offset")
	errPayloadTooSmall                 = errors.New("2110-41 payload too small")
)
