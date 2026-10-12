// HermesHub 平台服务端
// 用法: hermeshub-server [--env .env]
package main

import (
	"fmt"
	"os"

	"hermeshub/internal/server"
)

func main() {
	if err := server.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "server error:", err)
		os.Exit(1)
	}
}
