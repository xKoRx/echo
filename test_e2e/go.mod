module github.com/xKoRx/echo/test_e2e

go 1.25

require (
	github.com/stretchr/testify v1.10.0
	github.com/xKoRx/echo/core v0.0.0
	github.com/xKoRx/echo/agent v0.0.0
	github.com/xKoRx/echo/sdk v0.0.0
)

replace (
	github.com/xKoRx/echo/core => ../core
	github.com/xKoRx/echo/agent => ../agent
	github.com/xKoRx/echo/sdk => ../sdk
)

