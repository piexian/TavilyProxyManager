# Stage 1: Build the frontend
FROM node:20-alpine AS frontend-builder
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG ALL_PROXY
ARG NO_PROXY
ENV HTTP_PROXY=${HTTP_PROXY}
ENV HTTPS_PROXY=${HTTPS_PROXY}
ENV ALL_PROXY=${ALL_PROXY}
ENV NO_PROXY=${NO_PROXY}
WORKDIR /web
COPY web/package*.json ./
RUN npm install
COPY web/ .
RUN npm run build

# Stage 2: Build the backend
FROM golang:1.23-alpine AS backend-builder
ARG GOPROXY=https://proxy.golang.org,direct
ARG GOSUMDB=sum.golang.org
ARG HTTP_PROXY
ARG HTTPS_PROXY
ARG ALL_PROXY
ARG NO_PROXY
ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}
ENV HTTP_PROXY=${HTTP_PROXY}
ENV HTTPS_PROXY=${HTTPS_PROXY}
ENV ALL_PROXY=${ALL_PROXY}
ENV NO_PROXY=${NO_PROXY}
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Remove existing public files and copy built frontend
RUN rm -rf server/public/*
COPY --from=frontend-builder /web/dist/ server/public/
RUN go build -o tavily-proxy server/main.go

# Stage 3: Final image
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=backend-builder /app/tavily-proxy .

VOLUME /app/data
ENV DATABASE_PATH=/app/data/proxy.db

EXPOSE 8080

CMD ["./tavily-proxy"]
