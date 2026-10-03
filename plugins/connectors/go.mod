module github.com/raksha/raksha/plugins/connectors

go 1.26.4

require (
	github.com/raksha/raksha/core v1.6.2
	github.com/raksha/raksha/framework v1.4.2
)

replace (
	github.com/raksha/raksha/core => ../../core
	github.com/raksha/raksha/framework => ../../framework
)
