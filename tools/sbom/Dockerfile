FROM golang:1.18.7

WORKDIR /app

COPY tools/sbom/go.mod ./
COPY tools/sbom/go.sum ./

RUN go mod download

COPY tools/sbom/main.go ./

RUN go build -o /docker-sbom-generator

ENTRYPOINT ["/docker-sbom-generator", "-path", "/lifecycle"]