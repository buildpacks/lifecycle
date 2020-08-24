// +build !windows

package env

// Define only for compiler
func WindowsRegistryEnvMap() (map[string]string, error) {
	return nil, nil
}
