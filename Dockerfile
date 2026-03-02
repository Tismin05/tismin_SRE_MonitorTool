# 构建阶段
FROM golang:1.25-alpine AS builder

WORKDIR /app

# 安装构建依赖
RUN apk add --no-cache git

# 复制源码
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 构建二进制文件
RUN CGO_ENABLED=0 GOOS=linux go build -o tisminSRETool ./cmd/tisminSRETool

# 运行阶段
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# 从构建阶段复制二进制
COPY --from=builder /app/tisminSRETool .
COPY --from=builder /app/configs ./configs

# 创建非 root 用户
RUN adduser -D -u 1000 appuser
USER appuser

# 暴露端口
EXPOSE 8080

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# 启动命令
CMD ["./tisminSRETool"]