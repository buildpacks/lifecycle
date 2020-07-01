package archive

func setUmask(newMask int) (oldMask int) {
	// Not implemented on Windows
	return 0
}
