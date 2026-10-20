package updater

import (
	"net/http"
	"net/url"
	"os"

	"github.com/candyflosssss/Anchor/internal/systemproxy"
)

var systemProxy = systemproxy.ReadSystem
var lookupEnv = os.Getenv
var errProxy = systemproxy.ErrInvalid

// Updates retain their existing discovery and bypass semantics. Supplier
// requests opt into the same resolver through a separate, fail-closed transport.
func proxyFor(r *http.Request) (*url.URL, error) {
	return systemproxy.Resolve(r, lookupEnv, systemProxy)
}
func pickSystemProxy(server, override string, target *url.URL) (*url.URL, error) {
	return systemproxy.Pick(server, override, target)
}
