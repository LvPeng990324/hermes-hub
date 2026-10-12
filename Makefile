APP_NAME := hermeshub
GOFLAGS  := -ldflags="-s -w"

.PHONY: build web go server connector dev-web dev-server dev-connector clean

## build: 一键构建——前端打包 + 编译出两个二进制（服务端 / 连接器）
build: web go

## web: 构建前端产物 web/dist
web:
	cd web && npm install && npm run build

## go: 编译全部二进制（服务端嵌入前端产物，需先完成 web）
go: server connector

## server: 编译平台服务端 ./hermeshub-server（需先完成前端构建，产物经 go:embed 嵌入）
server:
	CGO_ENABLED=0 go build $(GOFLAGS) -o $(APP_NAME)-server ./cmd/server

## connector: 编译连接器 ./hermeshub-connector（独立二进制，仅部署到 Hermes 实例所在机器）
connector:
	CGO_ENABLED=0 go build $(GOFLAGS) -o $(APP_NAME)-connector ./cmd/connector

## dev-web: 前端开发模式（代理 /api /ws 到 :8080）
dev-web:
	cd web && npm run dev

## dev-server: 服务端开发模式
dev-server:
	go run ./cmd/server

## dev-connector: 连接器开发模式
dev-connector:
	go run ./cmd/connector --config connector.yaml

## clean: 清理构建产物
clean:
	rm -rf web/dist $(APP_NAME)-server $(APP_NAME)-connector
