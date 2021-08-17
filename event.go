package main

import (
	"fmt"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	liquiditytypes "github.com/gravity-devs/liquidity/x/liquidity/types"
	abcitypes "github.com/tendermint/tendermint/abci/types"
)

type Block struct {
	Height int64         `json:"height"`
	Events []interface{} `json:"events"`
}

type Event struct {
	Type       string            `json:"type"`
	Attributes map[string]string `json:"attributes"`
}

func NewEvent(event abcitypes.Event) Event {
	evt := Event{
		Type:       event.Type,
		Attributes: make(map[string]string),
	}
	for _, attr := range event.Attributes {
		evt.Attributes[string(attr.Key)] = string(attr.Value)
	}
	return evt
}

func (event *Event) Attr(key string) (string, error) {
	v, ok := event.Attributes[key]
	if !ok {
		return "", fmt.Errorf("attr %s not found", key)
	}
	return v, nil
}

func (event *Event) Uint64Attr(key string) (uint64, error) {
	s, err := event.Attr(key)
	if err != nil {
		return 0, err
	}
	i, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse uint64: %w", err)
	}
	return i, nil
}

func (event *Event) IntAttr(key string) (sdk.Int, error) {
	s, err := event.Attr(key)
	if err != nil {
		return sdk.Int{}, err
	}
	i, ok := sdk.NewIntFromString(s)
	if !ok {
		return sdk.Int{}, fmt.Errorf("not an Int: %s", s)
	}
	return i, nil
}

func (event *Event) DecAttr(key string) (sdk.Dec, error) {
	s, err := event.Attr(key)
	if err != nil {
		return sdk.Dec{}, err
	}
	d, err := sdk.NewDecFromStr(s)
	if err != nil {
		return sdk.Dec{}, err
	}
	return d, nil
}

func (event *Event) CoinAttrs(denomKey, amountKey string) (sdk.Coin, error) {
	denom, err := event.Attr(denomKey)
	if err != nil {
		return sdk.Coin{}, err
	}
	amount, err := event.IntAttr(amountKey)
	if err != nil {
		return sdk.Coin{}, err
	}
	return sdk.NewCoin(denom, amount), nil
}

func (event *Event) DecCoinAttrs(denomKey, amountKey string) (sdk.DecCoin, error) {
	denom, err := event.Attr(denomKey)
	if err != nil {
		return sdk.DecCoin{}, err
	}
	amount, err := event.DecAttr(amountKey)
	if err != nil {
		return sdk.DecCoin{}, err
	}
	return sdk.NewDecCoinFromDec(denom, amount), nil
}

type SwapTransactedEvent struct {
	Event
	Success                bool     `json:"success"`
	PoolID                 uint64   `json:"pool_id"`
	SwapRequesterAddress   string   `json:"swap_requester_address"`
	ExchangedOfferCoin     sdk.Coin `json:"exchanged_offer_coin"`
	ExchangedOfferCoinFee  sdk.Coin `json:"exchanged_offer_coin_fee"`
	ExchangedDemandCoin    sdk.Coin `json:"exchanged_demand_coin"`
	ExchangedDemandCoinFee sdk.Coin `json:"exchanged_demand_coin_fee"`
}

func NewSwapTransactedEvent(event abcitypes.Event) (SwapTransactedEvent, error) {
	evt := SwapTransactedEvent{Event: NewEvent(event)}
	success, err := evt.Attr(liquiditytypes.AttributeValueSuccess)
	if err != nil {
		return SwapTransactedEvent{}, err
	}
	evt.Success = success == liquiditytypes.Success
	evt.PoolID, err = evt.Uint64Attr(liquiditytypes.AttributeValuePoolId)
	if err != nil {
		return SwapTransactedEvent{}, err
	}
	evt.SwapRequesterAddress, err = evt.Attr(liquiditytypes.AttributeValueSwapRequester)
	if err != nil {
		return SwapTransactedEvent{}, err
	}
	evt.ExchangedOfferCoin, err = evt.CoinAttrs(liquiditytypes.AttributeValueOfferCoinDenom, liquiditytypes.AttributeValueExchangedOfferCoinAmount)
	if err != nil {
		return SwapTransactedEvent{}, err
	}
	if evt.Success {
		evt.ExchangedDemandCoin, err = evt.CoinAttrs(liquiditytypes.AttributeValueDemandCoinDenom, liquiditytypes.AttributeValueExchangedDemandCoinAmount)
		if err != nil {
			return SwapTransactedEvent{}, err
		}
		evt.ExchangedOfferCoinFee, err = evt.CoinAttrs(liquiditytypes.AttributeValueOfferCoinDenom, liquiditytypes.AttributeValueOfferCoinFeeAmount)
		if err != nil {
			return SwapTransactedEvent{}, err
		}
		feeDec, err := evt.DecCoinAttrs(liquiditytypes.AttributeValueDemandCoinDenom, liquiditytypes.AttributeValueExchangedCoinFeeAmount)
		if err != nil {
			return SwapTransactedEvent{}, err
		}
		evt.ExchangedDemandCoinFee = sdk.NewCoin(feeDec.Denom, feeDec.Amount.Ceil().TruncateInt())
	}
	return evt, nil
}
