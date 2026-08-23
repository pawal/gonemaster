//go:build nogui

package analysisui

import (
	"testing"

	"codeberg.org/pawal/gonemaster/server/internal/spatest"
)

func TestNoUIHandlerServesIndexInfoPage(t *testing.T) {
	spatest.NoUIIndexPage(t, Handler(""))
}

func TestNoUIHandlerReturnsNotFoundForAssets(t *testing.T) {
	spatest.NoUIAssetNotFound(t, Handler(""), "/assets/index.js")
}

func TestNoUIHandlerMethodNotAllowed(t *testing.T) {
	spatest.MethodNotAllowed(t, Handler(""))
}
