package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	liquiditytypes "github.com/gravity-devs/liquidity/x/liquidity/types"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

/*
Swap transacted heights: 6910473, 6910800, 6914057, 6914131
*/

func main() {
	var beginHeight, endHeight int64
	var outFileName string
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

			ctx := context.Background()

			if endHeight == 0 {
				latestBlockHeight, err := c.LatestBlockHeight(ctx)
				if err != nil {
					return fmt.Errorf("get latest block height: %w", err)
				}
				endHeight = latestBlockHeight
			}

			if beginHeight == 0 {
				beginHeight = endHeight - 999
				if beginHeight < 1 {
					beginHeight = 1
				}
			}

			if beginHeight > endHeight {
				return fmt.Errorf("begin height must be less or equal than end height")
			}

			fmt.Printf("begin height = %d, end height = %d\n", beginHeight, endHeight)

			query := fmt.Sprintf(`swap_transacted.pool_id EXISTS AND block.height >= %d AND block.height <= %d`, beginHeight, endHeight)
			heights, err := c.SearchBlockHeights(ctx, query)
			if err != nil {
				return fmt.Errorf("search block heights: %w", err)
			}
			fmt.Printf("searched heights = %v\n", heights)

			fmt.Print("continue? [y/N] ")
			var ans string
			if _, err := fmt.Scanln(&ans); err != nil {
				return fmt.Errorf("read input: %w", err)
			}
			if strings.ToLower(ans) != "y" {
				return fmt.Errorf("aborted")
			}

			outFile, err := os.OpenFile(outFileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("create out file: %w", err)
			}
			defer outFile.Close()

			fmt.Printf("writing output to %s\n", outFile.Name())

			bar := progressbar.Default(int64(len(heights)))

			for _, height := range heights {
				events, err := c.EndBlockEvents(ctx, height)
				if err != nil {
					return fmt.Errorf("get end block events: %w", err)
				}
				block := Block{Height: height, Events: []interface{}{}}
				for _, event := range events {
					if event.Type == liquiditytypes.EventTypeSwapTransacted {
						ste, err := NewSwapTransactedEvent(event)
						if err != nil {
							return fmt.Errorf("new swap_transacted event: %w", err)
						}
						block.Events = append(block.Events, ste)
					}
				}
				if err := json.NewEncoder(outFile).Encode(block); err != nil {
					return fmt.Errorf("write event to stdout: %w", err)
				}
				if err := outFile.Sync(); err != nil {
					return fmt.Errorf("sync out file: %w", err)
				}
				_ = bar.Add(1)
			}

			return nil
		},
	}
	cmd.Flags().Int64VarP(&beginHeight, "begin", "b", 0, "begin height")
	cmd.Flags().Int64VarP(&endHeight, "end", "e", 0, "end height")
	cmd.Flags().StringVarP(&outFileName, "outfile", "o", "output.log", "output file name")
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
