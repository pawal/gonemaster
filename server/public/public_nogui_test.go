//go:build nogui

package public

import (
	"testing"

	"codeberg.org/pawal/gonemaster/server/internal/spatest"
)

func TestNoUIHandlerServesIndexInfoPage(t *testing.T) {
	spatest.NoUIIndexPage(t, Handler(nil, "", nil))
}

func TestNoUIHandlerReturnsNotFoundForAssets(t *testing.T) {
	spatest.NoUIAssetNotFound(t, Handler(nil, "", nil), "/assets/index.js")
}

func TestNoUIHandlerMethodNotAllowed(t *testing.T) {
	spatest.MethodNotAllowed(t, Handler(nil, "", nil))
}
