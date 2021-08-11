package main

import (
	"context"
	"fmt"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	grpctypes "github.com/cosmos/cosmos-sdk/types/grpc"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	liquiditytypes "github.com/gravity-devs/liquidity/x/liquidity/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

type Client struct {
	cfg      ClientConfig
	grpcConn *grpc.ClientConn
}

func NewClient(cfg ClientConfig) (*Client, error) {
	grpcConn, err := grpc.Dial(cfg.GRPC.URL, grpc.WithTransportCredentials(credentials.NewTLS(nil)), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("dial grpc: %w", err)
	}
	return &Client{
		cfg:      cfg,
		grpcConn: grpcConn,
	}, nil
}

func (c *Client) Close() error {
	return c.grpcConn.Close()
}

func (c *Client) WithToken(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "Authorization", c.cfg.GRPC.Token)
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
	resp, err := lqc.LiquidityPools(c.WithToken(ctx), &liquiditytypes.QueryLiquidityPoolsRequest{}, grpc.Header(&md))
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
	resp, err := bqc.AllBalances(c.WithToken(ctx), &banktypes.QueryAllBalancesRequest{Address: addr}, grpc.Header(&md))
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
