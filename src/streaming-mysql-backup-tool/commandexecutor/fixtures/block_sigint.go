package main

import (
	"fmt"
	"os"
	"os/signal"
)

func main() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	for {
		select {
		case sig := <-sigCh:
			fmt.Printf("Caught: %s\n", sig)
		default:
			os.Stdout.Write([]byte("waiting to sigpipe\n"))
		}
	}
}
