package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	grpctypes "github.com/cosmos/cosmos-sdk/types/grpc"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	liquiditytypes "github.com/gravity-devs/liquidity/x/liquidity/types"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	rpc "github.com/tendermint/tendermint/rpc/client/http"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

type Client struct {
	cfg       ClientConfig
	grpcConn  *grpc.ClientConn
	rpcClient rpcclient.Client
}

func NewClient(cfg ClientConfig) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	opts := []grpc.DialOption{grpc.WithBlock()}
	if cfg.GRPC.Insecure {
		opts = append(opts, grpc.WithInsecure())
	} else {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(nil)))
	}
	grpcConn, err := grpc.DialContext(ctx, cfg.GRPC.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial grpc: %w", err)
	}
	httpClient := &http.Client{
		CheckRedirect: nil,
		Jar:           nil,
		Timeout:       0,
	}
	if cfg.RPC.Token != "" {
		httpClient.Transport = AddTokenRoundTripper{
			rt:    http.DefaultTransport,
			token: cfg.RPC.Token,
		}
	}
	rpcClient, err := rpc.NewWithClient(cfg.RPC.URL, "/websocket", httpClient)
	if err != nil {
		return nil, fmt.Errorf("new rpc client: %w", err)
	}
	return &Client{
		cfg:       cfg,
		grpcConn:  grpcConn,
		rpcClient: rpcClient,
	}, nil
}

func (c *Client) Close() error {
	return c.grpcConn.Close()
}

func (c *Client) withToken(ctx context.Context) context.Context {
	if c.cfg.GRPC.Token == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "Authorization", c.cfg.GRPC.Token)
}

func (c *Client) LatestBlockHeight(ctx context.Context) (int64, error) {
	resp, err := c.rpcClient.Status(ctx)
	if err != nil {
		return 0, err
	}
	return resp.SyncInfo.LatestBlockHeight, nil
}

func (c *Client) Pools(ctx context.Context, options ...ClientOption) ([]liquiditytypes.Pool, error) {
	opts := ClientOptions{}
	for _, opt := range options {
		opt(&opts)
	}
	if opts.blockHeight != nil {
		ctx = metadata.AppendToOutgoingContext(ctx, grpctypes.GRPCBlockHeightHeader, strconv.FormatInt(*opts.blockHeight, 10))
	}

	lqc := liquiditytypes.NewQueryClient(c.grpcConn)

	var md metadata.MD
	resp, err := lqc.LiquidityPools(c.withToken(ctx), &liquiditytypes.QueryLiquidityPoolsRequest{}, grpc.Header(&md))
	if err != nil {
		return nil, err
	}

	if opts.blockHeight != nil {
		if err := CheckBlockHeight(md, *opts.blockHeight); err != nil {
			return nil, fmt.Errorf("check block height: %w", err)
		}
	}

	return resp.Pools, nil
}

func (c *Client) AllBalances(ctx context.Context, addr string, options ...ClientOption) (sdk.Coins, error) {
	opts := ClientOptions{}
	for _, opt := range options {
		opt(&opts)
	}
	if opts.blockHeight != nil {
		ctx = metadata.AppendToOutgoingContext(ctx, grpctypes.GRPCBlockHeightHeader, strconv.FormatInt(*opts.blockHeight, 10))
	}

	bqc := banktypes.NewQueryClient(c.grpcConn)

	var md metadata.MD
	resp, err := bqc.AllBalances(c.withToken(ctx), &banktypes.QueryAllBalancesRequest{Address: addr}, grpc.Header(&md))
	if err != nil {
		return nil, err
	}

	if opts.blockHeight != nil {
		if err := CheckBlockHeight(md, *opts.blockHeight); err != nil {
			return nil, fmt.Errorf("check block height: %w", err)
		}
	}

	return resp.Balances, nil
}

func (c *Client) Balance(ctx context.Context, addr, denom string, options ...ClientOption) (sdk.Coin, error) {
	opts := ClientOptions{}
	for _, opt := range options {
		opt(&opts)
	}
	if opts.blockHeight != nil {
		ctx = metadata.AppendToOutgoingContext(ctx, grpctypes.GRPCBlockHeightHeader, strconv.FormatInt(*opts.blockHeight, 10))
	}

	bqc := banktypes.NewQueryClient(c.grpcConn)

	var md metadata.MD
	resp, err := bqc.Balance(c.withToken(ctx), &banktypes.QueryBalanceRequest{Address: addr, Denom: denom}, grpc.Header(&md))
	if err != nil {
		return sdk.Coin{}, err
	}

	if opts.blockHeight != nil {
		if err := CheckBlockHeight(md, *opts.blockHeight); err != nil {
			return sdk.Coin{}, fmt.Errorf("check block height: %w", err)
		}
	}

	return *resp.Balance, nil
}

func (c *Client) BlockTime(ctx context.Context, height int64) (time.Time, error) {
	resp, err := c.rpcClient.Block(ctx, &height)
	if err != nil {
		return time.Time{}, err
	}
	return resp.Block.Time, nil
}

func (c *Client) SearchBlockHeightByTime(ctx context.Context, t time.Time) (int64, error) {
	endHeight, err := c.LatestBlockHeight(ctx)
	if err != nil {
		return 0, fmt.Errorf("get latest block height: %w", err)
	}

	h := sort.Search(int(endHeight), func(h int) bool {
		if h < 5200791 { // TODO: remove this hard coded minimum height
			return false
		}
		t2, err := c.BlockTime(ctx, int64(h))
		if err != nil {
			panic(err)
		}
		return t2.After(t)
	})
	return int64(h), nil
}

func (c *Client) SearchBlockHeights(ctx context.Context, query string) ([]int64, error) {
	pageSize := 100
	maxPage := -1
	var heights []int64
	for page := 1; maxPage == -1 || page <= maxPage; page++ {
		resp, err := c.rpcClient.BlockSearch(ctx, query, &page, &pageSize, "asc")
		if err != nil {
			return nil, err
		}
		if resp.TotalCount == 0 {
			break
		}
		for _, block := range resp.Blocks {
			heights = append(heights, block.Block.Height)
		}
		if maxPage == -1 {
			maxPage = int(math.Ceil(float64(resp.TotalCount) / float64(pageSize)))
		}
	}
	return heights, nil
}

func (c *Client) EndBlockEvents(ctx context.Context, blockHeight int64) ([]abcitypes.Event, error) {
	resp, err := c.rpcClient.BlockResults(ctx, &blockHeight)
	if err != nil {
		return nil, err
	}
	return resp.EndBlockEvents, nil
}

type ClientOptions struct {
	blockHeight *int64
}

type ClientOption func(*ClientOptions)

func WithBlockHeight(blockHeight int64) ClientOption {
	return func(opts *ClientOptions) {
		opts.blockHeight = &blockHeight
	}
}

func CheckBlockHeight(md metadata.MD, height int64) error {
	vs := md.Get(grpctypes.GRPCBlockHeightHeader)
	if len(vs) < 1 {
		return fmt.Errorf("block height header not found")
	}
	h, err := strconv.ParseInt(vs[0], 10, 64)
	if err != nil {
		return fmt.Errorf("parse block height header: %w", err)
	}
	if h != height {
		return fmt.Errorf("mismatching block height; got %d, expected %d", h, height)
	}
	return nil
}

type AddTokenRoundTripper struct {
	rt    http.RoundTripper
	token string
}

func (rt AddTokenRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Add("Authorization", rt.token)
	return rt.rt.RoundTrip(req)
}
