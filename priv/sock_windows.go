package priv

import "net/url"

// shouldConnectSock is always false on windows
func shouldConnectSock(host *url.URL) bool {
	return false
}
