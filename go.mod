module github.com/allinbits/gravity-dex-stats

go 1.16

replace github.com/gogo/protobuf => github.com/regen-network/protobuf v1.3.3-alpha.regen.1

require (
	github.com/cosmos/cosmos-sdk v0.42.6
	github.com/gravity-devs/liquidity v1.2.9
	github.com/spf13/cobra v1.1.3
	github.com/spf13/viper v1.7.1
	github.com/tendermint/tendermint v0.34.11
	google.golang.org/grpc v1.39.1
)
