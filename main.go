package main

import (
	"fmt"
	"os"
	"syscall"
	"os/signal"
)

func main() {
	fmt.Println("Fake Device Plugin Running")
	devicePlugin := NewFakeDevicePlugin()
	devicePlugin.Serve()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	
	s := <- sig
	fmt.Println("Received signal \"%v\"", s)
}
