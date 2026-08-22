FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN --mount=type=secret,id=gouser \
    --mount=type=secret,id=gopat \
    GONOSUMDB=artifacts.ansonallard.com \
    GOPROXY=https://$(cat /run/secrets/gouser):$(cat /run/secrets/gopat)@artifacts.ansonallard.com/api/packages/ansonallard/go,https://proxy.golang.org,direct \
    go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/service_binary {{.ServiceBinaryDir}} && \
    mkdir -p /app/logs

FROM {{.FinalBaseImage}}

{{if .IncludeDockerCompose}}
COPY --from=docker/compose-bin:v2.29.7 /docker-compose /usr/local/lib/docker/cli-plugins/docker-compose
{{end}}

COPY --from=builder /app/service_binary /service_binary

COPY --from=builder /app/logs /logs

ENTRYPOINT ["/service_binary"]
