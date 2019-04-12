package cache

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle"
)

type VolumeCache struct {
	logger       *log.Logger
	dir          string
	backupDir    string
	stagingDir   string
	committedDir string
}

func NewVolumeCache(logger *log.Logger, dir string) (*VolumeCache, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, err
	}

	c := &VolumeCache{
		logger:       logger,
		dir:          dir,
		backupDir:    filepath.Join(dir, "committed-backup"),
		stagingDir:   filepath.Join(dir, "staging"),
		committedDir: filepath.Join(dir, "committed"),
	}

	if err := c.setupStagingDir(); err != nil {
		return nil, errors.Wrapf(err, "initializing staging directory '%s'", c.stagingDir)
	}

	if err := os.RemoveAll(c.backupDir); err != nil {
		return nil, errors.Wrapf(err, "removing backup directory '%s'", c.backupDir)
	}

	if err := os.MkdirAll(c.committedDir, 0777); err != nil {
		return nil, errors.Wrapf(err, "creating committed directory '%s'", c.committedDir)
	}

	return c, nil
}

func (c *VolumeCache) Name() string {
	return c.dir
}

func (c *VolumeCache) SetMetadata(metadata lifecycle.CacheMetadata) error {
	metadataPath := filepath.Join(c.stagingDir, lifecycle.CacheMetadataLabel)
	file, err := os.Create(metadataPath)
	if err != nil {
		return errors.Wrapf(err, "creating metadata file '%s'", metadataPath)
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(metadata); err != nil {
		return errors.Wrap(err, "marshalling metadata")
	}

	return nil
}

func (c *VolumeCache) RetrieveMetadata() (lifecycle.CacheMetadata, bool, error) {
	metadataPath := filepath.Join(c.committedDir, lifecycle.CacheMetadataLabel)
	file, err := os.Open(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return lifecycle.CacheMetadata{}, false, nil
		}
		return lifecycle.CacheMetadata{}, false, errors.Wrapf(err, "opening metadata file '%s'", metadataPath)
	}
	defer file.Close()

	var metadata lifecycle.CacheMetadata
	if json.NewDecoder(file).Decode(&metadata) != nil {
		c.logger.Printf("WARNING: cache has malformed metadata\n")
		return lifecycle.CacheMetadata{}, false, nil
	}

	return metadata, true, nil
}

func (c *VolumeCache) AddLayer(identifier string, sha string, tarPath string) error {
	c.logger.Printf("adding layer '%s' with diffID '%s'\n", identifier, sha)
	if err := copyFile(tarPath, filepath.Join(c.stagingDir, sha+".tar")); err != nil {
		return errors.Wrapf(err, "caching layer '%s' (%s)", identifier, sha)
	}
	return nil
}

func (c *VolumeCache) ReuseLayer(identifier string, sha string) error {
	c.logger.Printf("reusing layer '%s' with diffID '%s'\n", identifier, sha)
	if err := copyFile(filepath.Join(c.committedDir, sha+".tar"), filepath.Join(c.stagingDir, sha+".tar")); err != nil {
		return errors.Wrapf(err, "reusing layer '%s' (%s)", identifier, sha)
	}
	return nil
}

func (c *VolumeCache) RetrieveLayer(sha string) (io.ReadCloser, error) {
	file, err := os.Open(filepath.Join(c.committedDir, sha+".tar"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.Wrapf(err, "layer with SHA '%s' not found", sha)
		}
		return nil, errors.Wrapf(err, "retrieving layer with SHA '%s'", sha)
	}
	return file, nil
}

func (c *VolumeCache) Commit() error {
	c.logger.Println("committing cache")
	if err := os.Rename(c.committedDir, c.backupDir); err != nil {
		return errors.Wrap(err, "backing up cache")
	}
	defer os.RemoveAll(c.backupDir)

	if err := os.Rename(c.stagingDir, c.committedDir); err != nil {
		c.logger.Println("WARNING: failed to commit cache, attempting to roll back...")

		if err := os.Rename(c.backupDir, c.committedDir); err != nil {
			return errors.Wrap(err, "rolling back cache")
		}
		log.Println("successfully rolled back cache")
		return nil
	}

	return c.setupStagingDir()
}

func (c *VolumeCache) setupStagingDir() error {
	if err := os.RemoveAll(c.stagingDir); err != nil {
		return err
	}
	return os.MkdirAll(c.stagingDir, 0777)
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(to)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)

	return err
}
