package priv

import (
	"context"
	"net"
	"net/url"
	"os"

	"github.com/docker/docker/client"
	"github.com/pkg/errors"
)

// DockerClient constructs a client that can continue to talk to a root owned docker socket
// * even after the process drops privileges
func DockerClient() (*client.Client, error) {
	host := client.DefaultDockerHost
	if envHost := os.Getenv("DOCKER_HOST"); envHost != "" {
		host = envHost
	}
	hostURL, err := url.Parse(host)
	if err != nil {
		return nil, err
	}
	opts := []client.Opt{
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	}
	if shouldConnectSock(hostURL) {
		opt, err := connectSockOpt(hostURL)
		if err != nil {
			return nil, errors.Wrap(err, "failed to connect to docker socket")
		}
		opts = append(opts, opt)
	}
	return client.NewClientWithOpts(opts...)
}

type unclosableConn struct {
	net.Conn
}

func (*unclosableConn) Close() error {
	// don't close a connection we won't have privileges to reopen
	return nil
}

// connectSockOpt overrides the client dialer to always return the same socket connection
func connectSockOpt(host *url.URL) (client.Opt, error) {
	socketConn, err := net.Dial("unix", host.Path)
	if err != nil {
		return nil, err
	}
	return client.WithDialContext(func(ctx context.Context, network, addr string) (conn net.Conn, err error) {
		return &unclosableConn{socketConn}, nil
	}), nil
}
