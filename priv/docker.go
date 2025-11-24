package priv

import (
	"context"
	"net"
	"net/url"
	"os"
	"sync/atomic"

	"github.com/moby/moby/client"
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
	inUse int32
}

func (c *unclosableConn) Close() error {
	atomic.StoreInt32(&c.inUse, 0)
	// don't close a connection we won't have privileges to reopen
	return nil
}

// connectSockOpt overrides the client dialer to always return the same socket connection
func connectSockOpt(host *url.URL) (client.Opt, error) {
	socketConn, err := net.Dial("unix", host.Path)
	if err != nil {
		return nil, err
	}
	c := &unclosableConn{Conn: socketConn, inUse: 0}
	return client.WithDialContext(func(ctx context.Context, network, addr string) (conn net.Conn, err error) {
		if !atomic.CompareAndSwapInt32(&c.inUse, 0, 1) {
			return nil, errors.New("singleton connection already in use")
		}
		return c, nil
	}), nil
}
