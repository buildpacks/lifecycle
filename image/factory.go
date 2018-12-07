package image

import (
	"context"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	"io"
	"log"
	"os"

	"github.com/buildpack/lifecycle/fs"
	"github.com/docker/docker/api/types"
)

type Image interface {
	Label(string) (string, error)
	Rename(name string)
	Name() string
	Digest() (string, error)
	Rebase(string, Image) error
	SetLabel(string, string) error
	SetEnv(string, string) error
	TopLayer() (string, error)
	AddLayer(path string) error
	ReuseLayer(sha string) error
	Save() (string, error)
	Found() (bool, error)
}

type Docker interface {
	ImageInspectWithRaw(ctx context.Context, imageID string) (types.ImageInspect, []byte, error)
	ImageBuild(ctx context.Context, buildContext io.Reader, options types.ImageBuildOptions) (types.ImageBuildResponse, error)
	ImageRemove(ctx context.Context, ref string, options types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error)
	ImageLoad(ctx context.Context, r io.Reader, quiet bool) (types.ImageLoadResponse, error)
	ImageSave(ctx context.Context, imageIDs []string) (io.ReadCloser, error)
}

type Factory struct {
	Docker *client.Client
	Log    *log.Logger
	Stdout io.Writer
	FS     *fs.FS
}

func DefaultFactory() (*Factory, error) {
	f := &Factory{
		Stdout: os.Stdout,
		Log:    log.New(os.Stdout, "", log.LstdFlags),
		FS:     &fs.FS{},
	}

	var err error
	f.Docker, err = newDocker()
	if err != nil {
		return nil, err
	}

	return f, nil
}

func newDocker() (*client.Client, error) {
	docker, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.38"))
	if err != nil {
		return nil, errors.Wrap(err, "new docker client")
	}
	return docker, nil
}
