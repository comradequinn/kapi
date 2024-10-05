FROM docker.io/library/golang:1.23.3-alpine3.20 AS build
ARG VERSION=unset
COPY . /
WORKDIR /cmd/example
RUN go build -ldflags "-X 'main.Version=${VERSION}'" -o /bin/kapi-example

FROM scratch
COPY --from=build  /bin/kapi-example /
ENTRYPOINT [ "/kapi-example" ]
