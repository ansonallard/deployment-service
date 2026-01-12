FROM golang:1.24-alpine
WORKDIR /build

# Install dependencies
# RUN apk add --no-cache curl zip
RUN apk add curl zip

# Install oapi-codegen
RUN go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest

# Copy files
COPY go.mod .
COPY config.yaml .
COPY *.yaml ./

# Create output directory
RUN mkdir -p lib

# Generate Go client
RUN oapi-codegen -config=config.yaml bobawars-model.openapi.yaml

# Download dependencies to create go.sum
RUN go mod download

# Create the module zip with proper structure
# The zip should contain: module@version/go.mod, module@version/lib/client.go, etc.
RUN MODULE_DIR="artifacts.ansonallard.com/ansonallard/bobawars-go-client@v0.0.1" && \
    mkdir -p "$MODULE_DIR/lib" && \
    cp go.mod "$MODULE_DIR/" && \
    cp go.sum "$MODULE_DIR/" && \
    cp lib/* "$MODULE_DIR/lib/" && \
    zip -r module.zip "$MODULE_DIR"

# Debug: show zip contents
RUN unzip -l module.zip

# Upload to Gitea
RUN --mount=type=secret,id=gitea_token \
    curl --fail-with-body -X PUT \
    -H "Authorization: token $(cat /run/secrets/gitea_token)" \
    --upload-file /build/module.zip \
    "https://artifacts.ansonallard.com/api/packages/ansonallard/go/upload"

CMD ["sh", "-c", "echo 'Upload complete'"]