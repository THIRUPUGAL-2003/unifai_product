module github.com/gateway/gateway/plugins/connectors

go 1.26.4

require (
	github.com/gateway/gateway/core v1.6.2
	github.com/gateway/gateway/framework v1.4.2
)

replace (
	github.com/gateway/gateway/core => ../../core
	github.com/gateway/gateway/framework => ../../framework
)
