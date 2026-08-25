# BENZHI_README

基于 Go 实现的舞台布景阻燃准用工作台 Web 项目，一款后端服务，已完整实现舞台布景阻燃准用工作台：Go 服务提供响应式原生浏览器页面和同源 JSON API，以批次修订与状态机约束布景登记、方案冻结、逐项检查、缺陷整改、复测、安全批准及不可变准用凭据签发；本地版本化事务存储原子保存聚合、审计、幂等响应和摘要链凭据，并由真实 HTTP selfcheck 覆盖最小完整业务链路。

## 项目说明
- 项目：benzhi-project-bc80a7c5-3193-48a5-a69b-e5d5a5e4392b
- 项目用途：已完整实现舞台布景阻燃准用工作台：Go 服务提供响应式原生浏览器页面和同源 JSON API，以批次修订与状态机约束布景登记、方案冻结、逐项检查、缺陷整改、复测、安全批准及不可变准用凭据签发；本地版本化事务存储原子保存聚合、审计、幂等响应和摘要链凭据，并由真实 HTTP selfcheck 覆盖最小完整业务链路。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/scenicpermit -selfcheck -addr=127.0.0.1:19081
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-bc80a7c5-3193-48a5-a69b-e5d5a5e4392b-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-bc80a7c5-3193-48a5-a69b-e5d5a5e4392b-arm64 linux/arm64
docker run -it benzhi-project-bc80a7c5-3193-48a5-a69b-e5d5a5e4392b-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/scenicpermit -selfcheck -addr=127.0.0.1:19081`
