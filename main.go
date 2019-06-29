package main

import (
	"net"
	"net/rpc"
	"flag"
)

var (
	flagConfigPath string
)

func main() {
	flagReload := flag.NewFlagSet("reload", flag.ContinueOnError)

	flag.StringVar(&flagConfigPath, "config", defaultConfigPath, "configure path")
	flag.Parse()
	
	loadConfig(flagConfigPath)

	if flagReload.Parsed() {
		rc, err := rpc.Dial(config.RPC.Network, config.RPC.Address)
		if err != nil {
			panic(err)
		}
		defer rc.Close()

		err = rc.Call("Remote.Reload", nil, nil)
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

	go startDNSServer()
	go refreshCdn()
	httpServer.Start()

	remote := new(RPCRemote)
	rpc.RegisterName("remote", remote)

	rpc.Accept(rpcListener)
}

type RPCRemote struct {
}
func (r *RPCRemote) Reload(arg interface{}, reply *interface{}) error {
	loadConfig(flagConfigPath)
	httpServer.Restart()

	return nil
}