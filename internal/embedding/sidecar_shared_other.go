//go:build windows

package embedding

import "errors"

func newSharedSidecarAdapter(_ string, _ sidecarSpec) (Embedder, error) {
	return nil, errors.New("shared embedding sidecar is unsupported on this platform")
}
