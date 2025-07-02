FROM golang:1.24 AS builder

LABEL maintainer="a11199"
LABEL description="Base image loci dev"

WORKDIR /app

ENV GOOS=linux
ENV GOARCH=amd64

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN echo "Contents of /app after COPY:" && ls -al /app && sleep 1
RUN echo "Listing contents of /app after COPY:" && ls -al /app && sleep 1
RUN echo "Contents of /app/config:" && ls -al /app/config || echo "/app/config does not exist" && sleep 1
RUN ls -al /app/internal || echo "No /app/internal found" && sleep 1
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/loci -ldflags="-w -s" ./*.go
ENTRYPOINT ["/app/loci"]

# Development stage with hot reload
FROM golang:1.24 AS dev
WORKDIR /app
RUN go install github.com/air-verse/air@latest
COPY go.mod go.sum ./
RUN go mod download
COPY . .
EXPOSE 8000 8080 9090 6060
CMD ["air"]

#FROM golang:1.23-alpine AS debug
#WORKDIR /app
#
## Install Delve
#RUN go install github.com/go-delve/delve/cmd/dlv@latest
#
#COPY go.mod go.sum ./
#RUN go mod download
#
## Copy the application source code
#COPY . .
#
## Expose the debugging port for Delve
#EXPOSE 40000
#EXPOSE 8000
#
## Set the Delve command for debugging
#CMD ["dlv", "debug", "--headless", "--listen=:40000", "--api-version=2", "--accept-multiclient", "--log", "--", "--port=8000"]


FROM alpine:latest AS prod
WORKDIR /app

RUN apk add --no-cache bash
COPY --from=builder /app/loci /usr/bin/loci
COPY --from=builder /app/config ./config

RUN echo "Contents of /app after COPY:" && ls -al /app && sleep 1
RUN echo "Listing contents of /app after COPY:" && ls -al /app && sleep 1
RUN echo "Contents of /app/config:" && ls -al /app/config || echo "/app/config does not exist" && sleep 1
RUN ls -al /app/internal || echo "No /app/internal found" && sleep 1

EXPOSE 8000
EXPOSE 8001
CMD ["loci", "start"]
