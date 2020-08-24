package env

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// WindowsRegistryEnvMap returns a maps of Windows registry environment variabes
// All keys are upper-cased for equivalence with os.Getenv
func WindowsRegistryEnvMap() (map[string]string, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`, registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer key.Close()

	ki, err := key.Stat()
	if err != nil {
		return nil, err
	}

	regKeys, err := key.ReadValueNames(int(ki.ValueCount))
	if err != nil {
		return nil, err
	}

	regKeyValues := map[string]string{}
	for _, regKey := range regKeys {
		regValue, _, err := key.GetStringValue(regKey)
		if err != nil {
			return nil, err
		}

		regExpandedValue, err := registry.ExpandString(regValue)
		if err != nil {
			return nil, err
		}

		// set keys to upper for Getenv parity
		regKeyValues[strings.ToUpper(regKey)] = regExpandedValue
	}

	return regKeyValues, nil
}
