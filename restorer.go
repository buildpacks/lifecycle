package lifecycle

import (
	"log"

	"github.com/buildpack/lifecycle/fs"
	"github.com/buildpack/lifecycle/image"
)

type Restorer struct {
	LayersDir  string
	Buildpacks []*Buildpack
	Out, Err   *log.Logger
}

func (r *Restorer) Restore(cacheImage image.Image) error {
	if found, err := cacheImage.Found(); !found || err != nil {
		r.Out.Printf("cache image '%s' not found, nothing to restore", cacheImage.Name())
		return nil
	}
	metadata, err := getMetadata(cacheImage, r.Out)
	if err != nil {
		return err
	}
	archiver := &fs.FS{}
	for _, bp := range r.Buildpacks {
		layersDir, err := readBuildpackLayersDir(r.LayersDir, *bp)
		if err != nil {
			return err
		}
		bpMD := metadata.metadataForBuildpack(bp.ID)
		for name, layer := range bpMD.Layers {
			if !layer.Cache {
				continue
			}

			bpLayer := layersDir.newBPLayer(name)
			r.Out.Printf("restoring cached layer '%s'", bpLayer.Identifier())
			if err := bpLayer.writeMetadata(bpMD.Layers); err != nil {
				return err
			}
			if layer.Launch {
				if err := bpLayer.writeSha(layer.SHA); err != nil {
					return err
				}
			}
			rc, err := cacheImage.GetLayer(layer.SHA)
			if err != nil {
				return err
			}
			defer rc.Close()
			if err := archiver.Untar(rc, "/"); err != nil {
				return err
			}
		}
	}
	return nil
}
