package docker

import (
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
)

func DefaultClient() (*client.Client, error) {
	docker, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	if err != nil {
		return nil, errors.Wrap(err, "new docker client")
	}
	return docker, nil
}
