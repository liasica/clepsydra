# syntax=docker/dockerfile:1

# 阶段一：构建前端产物（dashboard/），先装依赖再拷贝源码以命中 pnpm store 缓存层
FROM node:24-alpine AS dashboard-builder
WORKDIR /src/dashboard

# package.json 未声明 packageManager 字段，显式固定 pnpm 版本，不依赖 corepack 默认值
RUN corepack enable && corepack prepare pnpm@11.20.0 --activate

COPY dashboard/package.json dashboard/pnpm-lock.yaml dashboard/pnpm-workspace.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

COPY dashboard/. .
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm build


# 阶段二：编译 Go 二进制，前端产物内嵌到 internal/api/static/dist 后一并编译
FROM golang:1.26-alpine AS server-builder
WORKDIR /src

# alpine 精简镜像不含证书，go mod download 走 https 需要
RUN apk add --no-cache ca-certificates git

ENV CGO_ENABLED=0

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
COPY --from=dashboard-builder /src/dashboard/dist/. internal/api/static/dist/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -o /out/clepsydra ./cmd/clepsydra


# 运行阶段：仅保留二进制与运行所需资源，业务重度依赖 Asia/Shanghai 日期计算，需固定时区
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=server-builder /out/clepsydra ./clepsydra
COPY assets ./assets

# 配置文件运行时挂载（如 -v ./configs:/app/configs），不随镜像分发
EXPOSE 8080
ENTRYPOINT ["/app/clepsydra"]
CMD ["-c", "configs/config.yaml"]
