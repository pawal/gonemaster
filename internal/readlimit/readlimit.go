// Package readlimit reads a stream that must not exceed a size.
package readlimit

import (
	"fmt"
	"io"
)

// ReadAll reads r to the end, failing when it holds more than max bytes.
func ReadAll(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("stream exceeds %d bytes", max)
	}
	return data, nil
}
