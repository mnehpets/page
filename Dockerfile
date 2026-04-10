FROM docker.io/golang:1.25 AS build
WORKDIR /source

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /app/pageserve ./cmd/pageserve

FROM docker.io/alpine AS final
WORKDIR /app
COPY --from=build /app/pageserve ./
COPY example/docker-default/pageserve.yaml ./

EXPOSE 8080

ENTRYPOINT ["/app/pageserve"]
CMD ["pageserve.yaml"]
