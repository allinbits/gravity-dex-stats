package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	liquiditytypes "github.com/gravity-devs/liquidity/x/liquidity/types"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

func RootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gravity-dex-stats",
		Short: "Gravity DEX statistics extractor",
	}
	cmd.AddCommand(
		SummaryCmd(),
		ReadGenesisCmd(),
		SearchBlockCmd(),
	)
	return cmd
}

type PoolSummary struct {
	ID           uint64
	ReserveCoins [2]sdk.Coin
	Swaps        [2]SwapSummary
}

type SwapSummary struct {
	OfferCoin     sdk.Coin
	OfferCoinFee  sdk.Coin
	DemandCoin    sdk.Coin
	DemandCoinFee sdk.Coin
}

func SummaryCmd() *cobra.Command {
	var beginHeight, endHeight int64
	var outFileName string
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Display short summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg, err := ReadClientConfig("config.toml")
			if err != nil {
				return fmt.Errorf("read client config: %w", err)
			}

			c, err := NewClient(cfg)
			if err != nil {
				return fmt.Errorf("new client: %w", err)
			}
			defer c.Close()

			ctx := context.Background()

			if endHeight == 0 {
				h, err := c.LatestBlockHeight(ctx)
				if err != nil {
					return fmt.Errorf("get latest block height: %w", err)
				}
				endHeight = h
			}

			if beginHeight > endHeight {
				return fmt.Errorf("begin height must be less or equal than end height")
			}

			fmt.Println("loading liquidity pools")

			pools, err := c.Pools(ctx, WithBlockHeight(endHeight))
			if err != nil {
				return fmt.Errorf("get pools: %w", err)
			}

			summaries := make(map[uint64]*PoolSummary)
			denomSet := make(map[string]struct{})
			swapRequesters := make(map[string]struct{})

			bar := progressbar.Default(int64(len(pools)))

			for _, pool := range pools {
				summaries[pool.Id] = &PoolSummary{ID: pool.Id}
				for i, denom := range pool.ReserveCoinDenoms {
					balance, err := c.Balance(ctx, pool.ReserveAccountAddress, denom, WithBlockHeight(endHeight))
					if err != nil {
						return fmt.Errorf("get balance: %w", err)
					}
					summaries[pool.Id].ReserveCoins[i] = balance
					summaries[pool.Id].Swaps[i].OfferCoin = sdk.NewCoin(denom, sdk.ZeroInt())
					summaries[pool.Id].Swaps[i].OfferCoinFee = sdk.NewCoin(denom, sdk.ZeroInt())
					summaries[pool.Id].Swaps[i].DemandCoin = sdk.NewCoin(pool.ReserveCoinDenoms[1-i], sdk.ZeroInt())
					summaries[pool.Id].Swaps[i].DemandCoinFee = sdk.NewCoin(pool.ReserveCoinDenoms[1-i], sdk.ZeroInt())
					denomSet[denom] = struct{}{}
				}
				_ = bar.Add(1)
			}

			fmt.Println("loading events")

			heights, err := c.SearchBlockHeights(
				ctx,
				fmt.Sprintf(`swap_transacted.pool_id EXISTS AND block.height >= %d AND block.height <= %d`, beginHeight, endHeight),
			)
			if err != nil {
				return fmt.Errorf("search block heights: %w", err)
			}
			if len(heights) == 0 {
				fmt.Println("no swap events found")
				return nil
			}

			bar = progressbar.Default(int64(len(heights)))

			for _, height := range heights {
				events, err := c.EndBlockEvents(ctx, height)
				if err != nil {
					return fmt.Errorf("get end block events: %w", err)
				}
				for _, event := range events {
					if event.Type == liquiditytypes.EventTypeSwapTransacted {
						ste, err := NewSwapTransactedEvent(event)
						if err != nil {
							return fmt.Errorf("new swap_transacted event: %w", err)
						}
						if ste.Success {
							ps, ok := summaries[ste.PoolID]
							if !ok {
								return fmt.Errorf("pool id not found: %d", ste.PoolID)
							}
							var i int
							if ste.ExchangedOfferCoin.Denom == ps.Swaps[0].OfferCoin.Denom {
								i = 0
							} else {
								i = 1
							}
							ps.Swaps[i].OfferCoin = ps.Swaps[i].OfferCoin.Add(ste.ExchangedOfferCoin)
							ps.Swaps[i].OfferCoinFee = ps.Swaps[i].OfferCoinFee.Add(ste.ExchangedOfferCoinFee)
							ps.Swaps[i].DemandCoin = ps.Swaps[i].DemandCoin.Add(ste.ExchangedDemandCoin)
							ps.Swaps[i].DemandCoinFee = ps.Swaps[i].DemandCoinFee.Add(ste.ExchangedDemandCoinFee)
							swapRequesters[ste.SwapRequesterAddress] = struct{}{}
						}
					}
				}
				_ = bar.Add(1)
			}

			fmt.Printf("Gravity DEX Summary (block height: %d)\n", endHeight)
			fmt.Printf("* %d kind(s) of token\n", len(denomSet))
			fmt.Printf("* %d swap trader(s)\n", len(swapRequesters))

			outFile, err := os.Create(outFileName)
			if err != nil {
				return fmt.Errorf("create output file: %w", err)
			}
			defer outFile.Close()

			csvWriter := csv.NewWriter(outFile)

			records := [][]string{{
				"id", "x_denom", "y_denom", "x", "y",
				"offer_x", "offer_x_fee", "demand_y", "demand_y_fee",
				"offer_y", "offer_y_fee", "demand_x", "demand_x_fee",
			}}
			for _, pool := range pools {
				ps := summaries[pool.Id]
				records = append(records, []string{
					strconv.FormatUint(ps.ID, 10),
					ps.ReserveCoins[0].Denom,
					ps.ReserveCoins[1].Denom,
					ps.ReserveCoins[0].Amount.String(),
					ps.ReserveCoins[1].Amount.String(),
					ps.Swaps[0].OfferCoin.Amount.String(),
					ps.Swaps[0].OfferCoinFee.Amount.String(),
					ps.Swaps[0].DemandCoin.Amount.String(),
					ps.Swaps[0].DemandCoinFee.Amount.String(),
					ps.Swaps[1].OfferCoin.Amount.String(),
					ps.Swaps[1].OfferCoinFee.Amount.String(),
					ps.Swaps[1].DemandCoin.Amount.String(),
					ps.Swaps[1].DemandCoinFee.Amount.String(),
				})
			}

			if err := csvWriter.WriteAll(records); err != nil {
				return fmt.Errorf("write output: %w", err)
			}

			return nil
		},
	}
	cmd.Flags().Int64VarP(&beginHeight, "begin", "b", 1, "Begin block height")
	cmd.Flags().Int64VarP(&endHeight, "end", "e", 0, "End block height")
	cmd.Flags().StringVarP(&outFileName, "out", "o", "pools.csv", "Output file name")
	return cmd
}

func ReadGenesisCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "read-genesis [file]",
		Short: "Read genesis file and extract summary",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			cfg, err := ReadClientConfig("config.toml")
			if err != nil {
				return fmt.Errorf("read client config: %w", err)
			}

			c, err := NewClient(cfg)
			if err != nil {
				return fmt.Errorf("new client: %w", err)
			}
			defer c.Close()

			ctx := context.Background()

			pools, err := c.Pools(ctx)
			if err != nil {
				return fmt.Errorf("get pools: %w", err)
			}

			poolCoinDenomSet := make(map[string]uint64) // (pool coin denom) => (pool id)
			for _, pool := range pools {
				poolCoinDenomSet[pool.PoolCoinDenom] = pool.Id
			}

			appState, _, err := genutiltypes.GenesisStateFromGenFile(args[0])
			if err != nil {
				return fmt.Errorf("genesis state from file: %w", err)
			}

			genState := banktypes.GetGenesisStateFromAppState(codec.NewProtoCodec(codectypes.NewInterfaceRegistry()), appState)

			numPoolInvestors := make(map[uint64]int) // (pool id) => (num pool investors)

			for _, balance := range genState.Balances {
				for poolCoinDenom, poolID := range poolCoinDenomSet {
					if balance.Coins.AmountOf(poolCoinDenom).IsPositive() {
						numPoolInvestors[poolID]++
					}
				}
			}

			fmt.Println("number of pool investors")
			fmt.Println("========================")
			for _, pool := range pools {
				fmt.Printf("pool %d: %d\n", pool.Id, numPoolInvestors[pool.Id])
			}

			return nil
		},
	}
	return cmd
}

func SearchBlockCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search-block [time]",
		Short: "Search block heights for specific time",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := time.Parse(time.RFC3339, args[0])
			if err != nil {
				return fmt.Errorf("parse time: %w", err)
			}

			cmd.SilenceUsage = true

			cfg, err := ReadClientConfig("config.toml")
			if err != nil {
				return fmt.Errorf("read client config: %w", err)
			}

			c, err := NewClient(cfg)
			if err != nil {
				return fmt.Errorf("new client: %w", err)
			}
			defer c.Close()

			ctx := context.Background()

			h, err := c.SearchBlockHeightByTime(ctx, t)
			if err != nil {
				return fmt.Errorf("search block height by time: %w", err)
			}

			t, err = c.BlockTime(ctx, h)
			if err != nil {
				return fmt.Errorf("get block time: %w", err)
			}

			fmt.Printf("Nearest block height is %d, time is %s\n", h, t.Format(time.RFC3339))

			return nil
		},
	}
	return cmd
}
