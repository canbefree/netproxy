package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"netproxy/pkg/client"
	"netproxy/pkg/common"
	"netproxy/pkg/config"
	"netproxy/pkg/service"
)

var (
	configFile string
)

var rootCmd = &cobra.Command{
	Use:   "netproxy",
	Short: "轻量级内网穿透工具",
	Long:  `NetProxy是一个用Go语言实现的轻量级内网穿透/反向代理工具，类似于FRP，可用于在任意网络上绕过NAT/防火墙，远程访问内网机器。`,
}

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "启动服务端",
	Long:  `启动服务端程序，监听客户端连接并转发流量。`,
	Run: func(cmd *cobra.Command, args []string) {
		// 加载配置文件
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmt.Printf("加载配置文件失败: %v\n", err)
			os.Exit(1)
		}

		// 启动服务端
		server := service.NewServer(cfg)
		common.Log("Starting server...")

		// 启动服务
		err = server.Start()
		if err != nil {
			common.Log("Server error: %v", err)
			os.Exit(1)
		}
	},
}

var clientCmd = &cobra.Command{
	Use:   "client",
	Short: "启动客户端",
	Long:  `启动客户端程序，连接到服务端并转发本地服务。`,
	Run: func(cmd *cobra.Command, args []string) {
		// 加载配置文件
		cfg, err := config.LoadConfig(configFile)
		if err != nil {
			fmt.Printf("加载配置文件失败: %v\n", err)
			os.Exit(1)
		}

		// 启动客户端
		cli := client.NewClient(cfg)
		common.Log("Starting client...")

		// 启动客户端
		err = cli.Start()
		if err != nil {
			common.Log("Client error: %v", err)
			os.Exit(1)
		}
	},
}

func init() {
	// 全局参数
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "config.json", "配置文件路径")

	// 添加子命令
	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(clientCmd)

	// 为了兼容旧版本，添加简写的命令行参数
	rootCmd.Flags().BoolP("server", "s", false, "启动服务端 (简写)")
	rootCmd.Flags().BoolP("client", "C", false, "启动客户端 (简写)")
}

func main() {
	// 检查简写参数
	if len(os.Args) > 1 {
		for i, arg := range os.Args {
			if arg == "-s" {
				// 替换为 server 命令
				os.Args[i] = "server"
			} else if arg == "-c" {
				// 如果不是配置文件参数，则替换为 client 命令
				if i == 1 || (i > 1 && os.Args[i-1] != "--config") {
					os.Args[i] = "client"
				}
			}
		}
	}

	// 执行命令
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
