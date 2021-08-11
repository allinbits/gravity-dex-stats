package main

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

func main() {
	cmd := &cobra.Command{
		Use: "gravity-dex-stats",
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

			pools, err := c.Pools(context.Background())
			if err != nil {
				return fmt.Errorf("get pools: %w", err)
			}

			for _, pool := range pools {
				balances, err := c.AllBalances(context.Background(), pool.ReserveAccountAddress)
				if err != nil {
					return fmt.Errorf("get all balances: %w", err)
				}
				fmt.Printf("pool %d has = %s\n", pool.Id, balances)
			}

			return nil
		},
	}
	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
