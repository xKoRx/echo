module github.com/xKoRx/echo/agent

go 1.23

require (
	github.com/xKoRx/echo/sdk v0.0.0
	google.golang.org/grpc v1.70.0
	google.golang.org/protobuf v1.36.4
)

replace github.com/xKoRx/echo/sdk => ../sdk

