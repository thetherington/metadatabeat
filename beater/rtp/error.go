// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package rtp

import (
	"errors"
)

var (
	errHeaderSizeInsufficient             = errors.New("RTP header size insufficient")
	errHeaderSizeInsufficientForExtension = errors.New("RTP header size insufficient for extension")
	errTooSmall                           = errors.New("buffer too small")
)
