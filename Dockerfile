FROM golang:1.22 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY internal ./internal
COPY pkg ./pkg
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o kube2kafka main.go

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /app/kube2kafka /kube2kafka
USER 65532:65532
ENTRYPOINT ["/kube2kafka"]