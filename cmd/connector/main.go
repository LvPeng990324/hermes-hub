// HermesHub 连接器（部署在 Hermes 实例所在机器）
// 用法:
//   hermeshub-connector --config connector.yaml   启动连接器
//   hermeshub-connector toolbridge                工具桥模式（由 hermes acp 会话拉起，勿手动执行）
package main

import (
	"fmt"
	"os"

	"hermeshub/internal/connector"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "toolbridge" {
		if err := connector.RunToolBridge(); err != nil {
			fmt.Fprintln(os.Stderr, "toolbridge error:", err)
			os.Exit(1)
		}
		return
	}
	if err := connector.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "connector error:", err)
		os.Exit(1)
	}
}
