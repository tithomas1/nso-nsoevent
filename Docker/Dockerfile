###########
# 1st stage - Compile app
###########

FROM tithomas/gowithpkgs as appbase

ARG build_date=unspecified
ARG APP

LABEL org.opencontainers.image.title="NSOEventStream" \
      org.opencontainers.image.description="Cisco NSO event streaming" \
      org.opencontainers.image.vendor="Cisco Systems" \
      org.opencontainers.image.created="${build_date}" \
      org.opencontainers.image.url="unspecified"

ENV HTTP_PROXY http://proxy.esl.cisco.com:8080
ENV HTTPS_PROXY http://proxy.esl.cisco.com:8080

# Copy over application code
#
# Note GOPATH in the official containers is /go
RUN mkdir -p /go/src/${app}
WORKDIR /go/src/${app}
# Make sure .dockerignore is up to date!
COPY . ./

# Newer versions of go (1.9.4+) have restricted which flags can be passed to
# the CGO compiler and linker by default. The restrictions can be overridden
# by CGO_*_ALLOW regular expressions. See https://golang.org/cmd/cgo
#ENV CGO_LDFLAGS_ALLOW '-.*'
ENV CGO_LDFLAGS_ALLOW '-fprofile-arcs|-ftest-coverage|--coverage'

# Static link. Use the built-in net package instead of the system one
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -tags netgo -installsuffix netgo -ldflags '-extldflags "-static"' -o main .


###########
# 2nd stage - Bare bones with app and any required assets
###########

FROM scratch
ARG app
WORKDIR /app
COPY config.yaml ./
COPY --from=appbase /go/src/${app}/main ./main

CMD ["./main"]
