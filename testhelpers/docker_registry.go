package testhelpers

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"testing"
	"time"

	"github.com/dgodd/dockerdial"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockercli "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type DockerRegistry struct {
	Port string
	Name string
}

func NewDockerRegistry() *DockerRegistry {
	return &DockerRegistry{
		Name: "test-registry-" + RandString(10),
	}
}

func (registry *DockerRegistry) Start(t *testing.T, seedRegistry bool) {
	t.Log("run registry")
	t.Helper()
	registry.Name = "test-registry-" + RandString(10)

	AssertNil(t, PullImage(DockerCli(t), "registry:2"))
	ctx := context.Background()
	ctr, err := DockerCli(t).ContainerCreate(ctx, &container.Config{
		Image: "registry:2",
	}, &container.HostConfig{
		AutoRemove: true,
		PortBindings: nat.PortMap{
			"5000/tcp": []nat.PortBinding{{}},
		},
	}, nil, registry.Name)
	AssertNil(t, err)
	defer DockerCli(t).ContainerRemove(ctx, ctr.ID, dockertypes.ContainerRemoveOptions{})
	err = DockerCli(t).ContainerStart(ctx, ctr.ID, dockertypes.ContainerStartOptions{})
	AssertNil(t, err)

	inspect, err := DockerCli(t).ContainerInspect(context.TODO(), ctr.ID)
	AssertNil(t, err)
	registry.Port = inspect.NetworkSettings.Ports["5000/tcp"][0].HostPort

	if os.Getenv("DOCKER_HOST") != "" {
		err := proxyDockerHostPort(DockerCli(t), registry.Port)
		AssertNil(t, err)
	}

	Eventually(t, func() bool {
		txt, err := HttpGetE(fmt.Sprintf("http://localhost:%s/v2/", registry.Port))
		return err == nil && txt != ""
	}, 100*time.Millisecond, 10*time.Second)

	if seedRegistry {
		t.Log("seed registry")
		for _, f := range []func(*testing.T, string) string{DefaultBuildImage, DefaultRunImage, DefaultBuilderImage} {
			AssertNil(t, pushImage(DockerCli(t), f(t, registry.Port)))
		}
	}
}

func proxyDockerHostPort(dockerCli *dockercli.Client, port string) error {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}

	go func() {
		// TODO exit somehow.
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Println(err)
				continue
			}
			go func(conn net.Conn) {
				defer conn.Close()
				c, err := dockerdial.Dial("tcp", "localhost:"+port)
				if err != nil {
					log.Println(err)
					return
				}
				defer c.Close()

				go io.Copy(c, conn)
				io.Copy(conn, c)
			}(conn)
		}
	}()
	return nil
}

func (registry *DockerRegistry) Stop(t *testing.T) {
	t.Log("stop registry")
	t.Helper()
	if registry.Name != "" {
		DockerCli(t).ContainerKill(context.Background(), registry.Name, "SIGKILL")
		DockerCli(t).ContainerRemove(context.TODO(), registry.Name, dockertypes.ContainerRemoveOptions{Force: true})
	}
}
