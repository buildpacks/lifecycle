package env

import (
	"os"
	"path/filepath"
	"strings"
)

// PrepareWindowsOSEnv merges map of Windows registry environment variables with in-memory environment vars
// Injects os.Getenv an os.Setenv
func PrepareWindowsOSEnv(regEnvMap map[string]string, getenv func(string) string, setenv func(string, string) error) error {
	for regKey, regValue := range regEnvMap {
		envValue := getenv(regKey)

		// exit early if values match
		if regValue == envValue {
			continue
		}

		newValue := ""

		if regKey == "PATH" || regKey == "PATHEXT" {
			newValue = mergePathVars(regValue, envValue)
		} else {
			// prefer env value if set
			if envValue != "" {
				continue
			}

			newValue = regValue
		}

		if err := setenv(regKey, newValue); err != nil {
			return err
		}
	}

	return nil
}

// merge PATH and PATH by prepending registry value to existing env value
func mergePathVars(regValue, envValue string) string {
	var newValueParts []string
	newValueMap := map[string]bool{}

	for _, regValuePart := range filepath.SplitList(regValue) {
		key := strings.ToLower(regValuePart)
		if regValuePart == "" || newValueMap[key] {
			continue
		}

		newValueMap[key] = true
		newValueParts = append(newValueParts, regValuePart)
	}

	for _, envValuePart := range filepath.SplitList(envValue) {
		key := strings.ToLower(envValuePart)
		if envValuePart == "" || newValueMap[key] {
			continue
		}
		newValueParts = append(newValueParts, envValuePart)
	}

	return strings.Join(newValueParts, string(os.PathListSeparator))
}
