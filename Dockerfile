FROM golang:1.22 as build

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY . ./

RUN go generate ./pkg/cmd/server.go
RUN go build -o ./scanoss-hfh ./cmd/server

FROM debian:buster-slim

WORKDIR /app
 
COPY --from=build /app/scanoss-hfh /app/scanoss-hfh

EXPOSE 50053

ENTRYPOINT ["./scanoss-hfh"]
#CMD ["--help"]
