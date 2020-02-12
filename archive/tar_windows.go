// +build windows

package archive

func setUmask(newMask int) (oldMask int) {
	panic("Not implemented on Windows")

	return -1
}
