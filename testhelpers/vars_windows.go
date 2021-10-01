package testhelpers

import "testing"

func GetUmask(t *testing.T) int {
	// Not implemented on Windows
	return 0
}
