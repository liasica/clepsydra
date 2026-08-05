# syntax=docker/dockerfile:1

# 阶段一：构建前端产物（dashboard/），先装依赖再拷贝源码以命中 pnpm store 缓存层
FROM node:24-alpine AS dashboard-builder
WORKDIR /src/dashboard

# 对齐 vben 根 package.json 声明的 packageManager 版本，避免 corepack 拉取其他版本
RUN corepack enable && corepack prepare pnpm@10.33.4 --activate

# vben 是 pnpm workspace（monorepo），--frozen-lockfile 需要全部子包的 package.json 在场
# 才能解析出正确的依赖图；按子包逐行 COPY 以便源码变动时仍命中本层 pnpm store 缓存
COPY dashboard/package.json dashboard/pnpm-lock.yaml dashboard/pnpm-workspace.yaml dashboard/.npmrc ./
COPY dashboard/apps/web-antdv-next/package.json apps/web-antdv-next/package.json
COPY dashboard/internal/lint-configs/commitlint-config/package.json internal/lint-configs/commitlint-config/package.json
COPY dashboard/internal/lint-configs/eslint-config/package.json internal/lint-configs/eslint-config/package.json
COPY dashboard/internal/lint-configs/oxfmt-config/package.json internal/lint-configs/oxfmt-config/package.json
COPY dashboard/internal/lint-configs/oxlint-config/package.json internal/lint-configs/oxlint-config/package.json
COPY dashboard/internal/lint-configs/stylelint-config/package.json internal/lint-configs/stylelint-config/package.json
COPY dashboard/internal/node-utils/package.json internal/node-utils/package.json
COPY dashboard/internal/tailwind-config/package.json internal/tailwind-config/package.json
COPY dashboard/internal/tsconfig/package.json internal/tsconfig/package.json
COPY dashboard/internal/vite-config/package.json internal/vite-config/package.json
COPY dashboard/packages/@core/base/design/package.json packages/@core/base/design/package.json
COPY dashboard/packages/@core/base/icons/package.json packages/@core/base/icons/package.json
COPY dashboard/packages/@core/base/shared/package.json packages/@core/base/shared/package.json
COPY dashboard/packages/@core/base/typings/package.json packages/@core/base/typings/package.json
COPY dashboard/packages/@core/composables/package.json packages/@core/composables/package.json
COPY dashboard/packages/@core/preferences/package.json packages/@core/preferences/package.json
COPY dashboard/packages/@core/ui-kit/form-ui/package.json packages/@core/ui-kit/form-ui/package.json
COPY dashboard/packages/@core/ui-kit/layout-ui/package.json packages/@core/ui-kit/layout-ui/package.json
COPY dashboard/packages/@core/ui-kit/menu-ui/package.json packages/@core/ui-kit/menu-ui/package.json
COPY dashboard/packages/@core/ui-kit/popup-ui/package.json packages/@core/ui-kit/popup-ui/package.json
COPY dashboard/packages/@core/ui-kit/shadcn-ui/package.json packages/@core/ui-kit/shadcn-ui/package.json
COPY dashboard/packages/@core/ui-kit/tabs-ui/package.json packages/@core/ui-kit/tabs-ui/package.json
COPY dashboard/packages/constants/package.json packages/constants/package.json
COPY dashboard/packages/effects/access/package.json packages/effects/access/package.json
COPY dashboard/packages/effects/common-ui/package.json packages/effects/common-ui/package.json
COPY dashboard/packages/effects/hooks/package.json packages/effects/hooks/package.json
COPY dashboard/packages/effects/layouts/package.json packages/effects/layouts/package.json
COPY dashboard/packages/effects/plugins/package.json packages/effects/plugins/package.json
COPY dashboard/packages/effects/request/package.json packages/effects/request/package.json
COPY dashboard/packages/icons/package.json packages/icons/package.json
COPY dashboard/packages/locales/package.json packages/locales/package.json
COPY dashboard/packages/preferences/package.json packages/preferences/package.json
COPY dashboard/packages/stores/package.json packages/stores/package.json
COPY dashboard/packages/styles/package.json packages/styles/package.json
COPY dashboard/packages/types/package.json packages/types/package.json
COPY dashboard/packages/utils/package.json packages/utils/package.json
COPY dashboard/scripts/turbo-run/package.json scripts/turbo-run/package.json
COPY dashboard/scripts/vsh/package.json scripts/vsh/package.json

# --ignore-scripts：根 package.json 的 postinstall（pnpm -r run stub --if-present）
# 会执行各子包的构建脚本，此时源码尚未拷贝进来，必须跳过，源码就位后再补跑
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile --ignore-scripts

COPY dashboard/. .

# 源码就位后补跑 stub，为内部包生成开发态产物（tsdown --if-present 等），等价于 postinstall
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm -r run stub --if-present
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm build --filter=@vben/web-antdv-next


# 阶段二：编译 Go 二进制，前端产物内嵌到 assets/dashboard 后一并编译
FROM golang:1.26-alpine AS server-builder
WORKDIR /src

# alpine 精简镜像不含证书，go mod download 走 https 需要
RUN apk add --no-cache ca-certificates git

ENV CGO_ENABLED=0

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
COPY --from=dashboard-builder /src/dashboard/dist/. assets/dashboard/

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
