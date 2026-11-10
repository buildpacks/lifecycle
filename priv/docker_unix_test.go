//go:build unix

package priv

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/moby/moby/client"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestSingletonConnectionSurvivesIdleTimeout(t *testing.T) {
	dir, err := os.MkdirTemp("", "priv")
	h.AssertNil(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "docker.sock")

	listener, err := net.Listen("unix", sock)
	h.AssertNil(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go serveSplitTerminator(listener)

	host := "unix://" + sock
	t.Setenv("DOCKER_HOST", host)
	hostURL, err := url.Parse(host)
	h.AssertNil(t, err)
	opt, err := connectSockOpt(hostURL)
	h.AssertNil(t, err)
	cli, err := client.New(client.FromEnv, opt)
	h.AssertNil(t, err)

	ctx := context.Background()
	_, err = cli.Info(ctx, client.InfoOptions{})
	h.AssertNil(t, err)

	time.Sleep(31 * time.Second)

	for range 2 {
		_, err = cli.Info(ctx, client.InfoOptions{})
		h.AssertNil(t, err)
	}
}

func serveSplitTerminator(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer func() { _ = conn.Close() }()
			reader := bufio.NewReader(conn)
			for {
				req, err := http.ReadRequest(reader)
				if err != nil {
					return
				}
				_ = req.Body.Close()
				if req.Method == http.MethodHead || filepath.Base(req.URL.Path) == "_ping" {
					_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nApi-Version: 1.41\r\nContent-Length: 0\r\n\r\n"))
					continue
				}
				body := `{"ID":"fake"}` + "\n"
				_, _ = fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nApi-Version: 1.41\r\nContent-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\n%x\r\n%s\r\n", len(body), body)
				time.Sleep(20 * time.Millisecond)
				_, _ = conn.Write([]byte("0\r\n\r\n"))
			}
		}()
	}
}
