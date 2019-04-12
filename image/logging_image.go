package image

import (
	"log"

	"github.com/pkg/errors"
)

type LoggingImage struct {
	Out   *log.Logger
	Image Image
}

func NewLoggingImage(out *log.Logger, image Image) *LoggingImage {
	return &LoggingImage{
		Out:   out,
		Image: image,
	}
}

func (li *LoggingImage) Name() string {
	return li.Image.Name()
}

func (li *LoggingImage) AddLayer(identifier, sha, tar string) error {
	li.Out.Printf("adding layer '%s' with diffID '%s'\n", identifier, sha)
	if err := li.Image.AddLayer(tar); err != nil {
		return errors.Wrapf(err, "add %s layer", identifier)
	}
	return nil
}

func (li *LoggingImage) ReuseLayer(identifier, sha string) error {
	li.Out.Printf("reusing layer '%s' with diffID '%s'\n", identifier, sha)
	if err := li.Image.ReuseLayer(sha); err != nil {
		return errors.Wrapf(err, "reuse %s layer", identifier)
	}
	return nil
}

func (li *LoggingImage) SetLabel(k string, v string) error {
	li.Out.Printf("setting metadata label '%s'\n", k)
	return li.Image.SetLabel(k, v)
}

func (li *LoggingImage) SetEnv(k string, v string) error {
	li.Out.Printf("setting env var '%s=%s'\n", k, v)
	return li.Image.SetEnv(k, v)
}

func (li *LoggingImage) SetEntrypoint(entryPoint string) error {
	li.Out.Printf("setting entrypoint '%s'\n", entryPoint)
	return li.Image.SetEntrypoint(entryPoint)
}

func (li *LoggingImage) SetEmptyCmd() error {
	li.Out.Println("setting empty cmd")
	return li.Image.SetCmd()
}

func (li *LoggingImage) Save() (string, error) {
	li.Out.Println("writing image")
	sha, err := li.Image.Save()
	return sha, err
}
