# BENZHI_README

基于 Go 实现的航测成果公开放行台 HTTP API 项目，一款后端服务，已完整实现航测成果公开放行台：通过单一 JSON HTTP 流程完成建档、校准核验、敏感要素整改、人工审批、清单冻结、凭据签发、审计查询与凭据验证，并具备可恢复的本地哈希链事件账本。

## 项目说明
- 项目：benzhi-project-851d159e-1761-4d2d-b0fb-746585cc770c
- 项目用途：已完整实现航测成果公开放行台：通过单一 JSON HTTP 流程完成建档、校准核验、敏感要素整改、人工审批、清单冻结、凭据签发、审计查询与凭据验证，并具备可恢复的本地哈希链事件账本。
- Go 工具链：`golang:1.23`
- 前端工具链：无

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/survey-release -addr=127.0.0.1:19081 -selfcheck -selfcheck-timeout=10s
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-851d159e-1761-4d2d-b0fb-746585cc770c-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-851d159e-1761-4d2d-b0fb-746585cc770c-arm64 linux/arm64
docker run -it benzhi-project-851d159e-1761-4d2d-b0fb-746585cc770c-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/survey-release -addr=127.0.0.1:19081 -selfcheck -selfcheck-timeout=10s`
