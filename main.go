package main

import (
	"flag"
	"net"
	"net/rpc"
	"os"
	
	"github.com/sirupsen/logrus"
)

var (
	flagConfigPath string
	logRusPanic	*logrus.Logger
)

type RpcArgs struct{}
type RpcResult struct{}

func main() {
	flag.StringVar(&flagConfigPath, "config", defaultConfigPath, "configure path")
	flag.Parse()
	
	loadConfig(flagConfigPath)
	
	logrus.SetFormatter(new(logrus.TextFormatter))
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.Level(config.LogLevel))

	logRusPanic = logrus.New()
	logRusPanic.SetFormatter(new(logrus.TextFormatter))
	logRusPanic.SetOutput(os.Stdout)
	logRusPanic.SetLevel(logrus.Level(config.LogLevel))
	logRusPanic.SetReportCaller(true)

	if len(os.Args) > 1 && os.Args[1] == "reload" {
		rc, err := rpc.Dial(config.RPC.Network, config.RPC.Address)
		if err != nil {
			panic(err)
		}
		defer rc.Close()

		err = rc.Call("Remote.Reload", new(RpcArgs), new(RpcResult))
		if err != nil {
			panic(err)
		}
		return
	}

	rpcListener, err := net.Listen(config.RPC.Network, config.RPC.Address)
	if err != nil {
		panic(err)
	}
	defer rpcListener.Close()

	cdnTester.Start()
	dnsServer.Start()
	httpServer.Start()

	rpc.RegisterName("Remote", new(RPCRemote))

	rpc.Accept(rpcListener)
}

type RPCRemote byte
func (r *RPCRemote) Reload(arg RpcArgs, reply *RpcResult) error {
	logrus.Info("reload configure")

	loadConfig(flagConfigPath)
	httpServer.Restart()

	return nil
}