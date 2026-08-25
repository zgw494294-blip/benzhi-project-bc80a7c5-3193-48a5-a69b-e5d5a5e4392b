package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultAddress = "127.0.0.1:19081"

type config struct {
	Address         string
	DataPath        string
	SelfCheck       bool
	ShutdownTimeout time.Duration
}

func parseConfig(args []string, lookupEnv func(string) (string, bool)) (config, error) {
	flags := flag.NewFlagSet("scenicpermit", flag.ContinueOnError)
	address := flags.String("addr", "", "HTTP 监听地址，例如 127.0.0.1:19081")
	dataPath := flags.String("data", "data/scenicpermit.json", "本地数据文件")
	selfCheck := flags.Bool("selfcheck", false, "运行有界 HTTP 全流程自检后退出")
	shutdownTimeout := flags.Duration("shutdown-timeout", 5*time.Second, "优雅关闭超时")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if flags.NArg() != 0 {
		return config{}, fmt.Errorf("不支持额外位置参数")
	}
	resolved := strings.TrimSpace(*address)
	if resolved == "" {
		if portText, ok := lookupEnv("PORT"); ok && strings.TrimSpace(portText) != "" {
			port, err := strconv.Atoi(strings.TrimSpace(portText))
			if err != nil || port < 1 || port > 65535 {
				return config{}, fmt.Errorf("PORT 必须是 1 到 65535 的端口号")
			}
			resolved = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		} else {
			resolved = defaultAddress
		}
	}
	host, portText, err := net.SplitHostPort(resolved)
	if err != nil {
		return config{}, fmt.Errorf("-addr 必须是 host:port：%w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return config{}, fmt.Errorf("监听端口无效")
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		return config{}, fmt.Errorf("拒绝未限定或全网监听地址，请显式使用回环地址")
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() {
		return config{}, fmt.Errorf("本工作台只允许绑定回环地址")
	}
	if *shutdownTimeout <= 0 || *shutdownTimeout > 30*time.Second {
		return config{}, fmt.Errorf("shutdown-timeout 必须在 0 到 30 秒之间")
	}
	return config{Address: resolved, DataPath: *dataPath, SelfCheck: *selfCheck, ShutdownTimeout: *shutdownTimeout}, nil
}

func environment(name string) (string, bool) { return os.LookupEnv(name) }
