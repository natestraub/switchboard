module github.com/switchboard

go 1.21

require (
	github.com/docker/docker v24.0.7+incompatible
	github.com/go-redis/redis/v8 v8.11.5
	go.uber.org/zap v1.27.0
)

replace github.com/docker/distribution => github.com/docker/distribution v2.8.2+incompatible

require (
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/docker/distribution v2.8.2+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/net v0.0.0-20210428140749-89ef3d95e781 // indirect
	golang.org/x/sys v0.1.0 // indirect
	golang.org/x/time v0.7.0 // indirect
	gotest.tools/v3 v3.5.1 // indirect
)
