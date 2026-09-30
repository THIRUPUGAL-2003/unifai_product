module github.com/unifai/unifai/plugins/connectors

go 1.26.4

require (
	github.com/unifai/unifai/core v1.6.2
	github.com/unifai/unifai/framework v1.4.2
)

replace (
	github.com/unifai/unifai/core => ../../core
	github.com/unifai/unifai/framework => ../../framework
)
