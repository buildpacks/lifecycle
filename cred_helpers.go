package lifecycle

import (
	"encoding/json"
	"fmt"
	"github.com/google/go-containerregistry/pkg/name"
	"os"
	"path/filepath"
	"regexp"
)

func SetupCredHelpers(home string, refs ...string) error {
	dockerPath := filepath.Join(home, ".docker")
	configPath := filepath.Join(dockerPath, "config.json")

	config := map[string]interface{}{}
	if f, err := os.Open(configPath); err == nil {
		err := json.NewDecoder(f).Decode(&config)
		if f.Close(); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if _, ok := config["credHelpers"]; !ok {
		config["credHelpers"] = make(map[string]interface{})
	}

	credHelpers := make(map[string]string)
	for _, refStr := range refs {
		ref, err := name.ParseReference(refStr, name.WeakValidation)
		if err != nil {
			return err
		}

		registry := ref.Context().RegistryStr()
		for _, ch := range []struct {
			domain string
			helper string
		}{
			{"([.]|^)gcr[.]io$", "gcr"},
			{"[.]amazonaws[.]", "ecr-login"},
			{"([.]|^)azurecr[.]io$", "acr"},
		} {
			match, err := regexp.MatchString("(?i)"+ch.domain, registry)
			if err != nil || !match {
				continue
			}
			credHelpers[registry] = ch.helper
		}
	}

	if len(credHelpers) == 0 {
		return nil
	}

	for k, v := range credHelpers {
		ch, ok := config["credHelpers"].(map[string]interface{})
		if !ok {
			return fmt.Errorf("failed to parse docker config 'credHelpers'")
		}

		if _, ok := ch[k]; ok {
			continue
		}

		ch[k] = v
	}

	if err := os.MkdirAll(dockerPath, 0777); err != nil {
		return err
	}

	f, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(config)
}
