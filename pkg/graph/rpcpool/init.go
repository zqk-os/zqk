package rpcpool

import (
	"net"
	"net/rpc"
	"net/rpc/jsonrpc"
	"os"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/graph/provider"
)

func StartServer(socketPath string, pool provider.ConnectionPool) error {
	server := rpc.NewServer()
	graphRPC := NewGraphRPC(pool)
	if err := server.RegisterName("GraphRPC", graphRPC); err != nil {
		return err
	}

	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}

	goroutinelabels.NewGoroutine("refactored_worker", "Refactored raw goroutine").
		StartSimple(func() {
			func() {
				for {
					conn, err := listener.Accept()
					if err != nil {
						return
					}
					go server.ServeCodec(jsonrpc.NewServerCodec(conn))
				}
			}()
		})
	return nil
}

func ConnectClient(socketPath string) (provider.ConnectionPool, error) {
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return nil, err
	}
	client := rpc.NewClientWithCodec(jsonrpc.NewClientCodec(conn))
	return NewRPCConnectionPool(client), nil
}
