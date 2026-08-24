# BENZHI_README

基于 Go 实现的地震台站仪器校准证书治理服务 Web 项目，一款后端服务，完整实现地震台站仪器校准证书治理服务，覆盖任务建档、测量计算、偏差整改、同行复核、退回修订、证书冻结签发、公开校验以及带哈希链的本地持久化恢复，并提供响应式中文 Web 工作台。

## 项目说明
- 项目：benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff
- 项目用途：完整实现地震台站仪器校准证书治理服务，覆盖任务建档、测量计算、偏差整改、同行复核、退回修订、证书冻结签发、公开校验以及带哈希链的本地持久化恢复，并提供响应式中文 Web 工作台。
- Go 工具链：`golang:1.23`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/server -addr=127.0.0.1:19081 -selfcheck
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff-arm64 linux/arm64
docker run -it benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -addr=127.0.0.1:19081 -selfcheck`
