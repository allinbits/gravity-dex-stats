import csv
import sys

import requests


def get_pools():
    url = "https://staging.demeris.io/v1/liquidity/cosmos/liquidity/v1beta1/pools"
    r = requests.get(url)
    return r.json()["pools"]


def get_balance(addr):
    url = "https://staging.demeris.io/v1/liquidity/cosmos/bank/v1beta1/balances/" + addr
    r = requests.get(url)
    return r.json()["balances"]


def get_verified_denoms():
    url = "https://staging.demeris.io/v1/verified_denoms"
    r = requests.get(url)
    return r.json()["verified_denoms"]


def get_prices():
    url = "https://staging.demeris.io/v1/oracle/prices"
    r = requests.get(url)
    return r.json()["data"]


def get_ibc_denom_info(denom):
    denom = denom.removeprefix("ibc/")
    url = "https://staging.demeris.io/v1/chain/cosmos-hub/denom/verify_trace/" + denom
    r = requests.get(url)
    if r.status_code != 200:
        return None
    return r.json()["verify_trace"]


def make_denom_price_map():
    denom_set = set()

    for pool in get_pools():
        denom_set |= set(pool["reserve_coin_denoms"])

    verified_denoms = get_verified_denoms()
    verified_denom_map = {x["name"]: x for x in verified_denoms}

    prices = get_prices()
    price_map = {x["Symbol"]: x["Price"] for x in prices["Tokens"]}

    denom_price_map = {}

    for denom in denom_set:
        if denom.startswith("ibc/"):
            info = get_ibc_denom_info(denom)
            if not info:
                continue
            base_denom = info["base_denom"]
        else:
            base_denom = denom

        denom_data = verified_denom_map[base_denom]
        if not denom_data["fetch_price"]:
            continue
        precision = denom_data["precision"]
        ticker = denom_data["ticker"]
        price = price_map[ticker + "USDT"]
        denom_price_map[denom] = price / pow(10, precision)

    return denom_price_map

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(f"usage: python3 {sys.argv[0]} [pools file]")
        sys.exit(0)

    denom_price_map = make_denom_price_map()

    tvl = 0.0
    for pool in get_pools():
        vl = 0.0
        try:
            for x in get_balance(pool["reserve_account_address"]):
                if x["denom"] in pool["reserve_coin_denoms"]:
                        vl += int(x["amount"]) * denom_price_map[x["denom"]]
        except KeyError:
            continue
        tvl += vl
    print(f"total value locked: {tvl}")

    swap_amount = 0.0
    fee_amount = 0.0

    with open(sys.argv[1], newline="") as f:
        reader = csv.DictReader(f)

        for row in reader:
            try:
                x_price = denom_price_map[row["x_denom"]]
                y_price = denom_price_map[row["y_denom"]]
            except KeyError:
                continue

            swap_amount += int(row["offer_x"]) * x_price
            swap_amount += int(row["offer_y"]) * y_price

            fee_amount += int(row["offer_x_fee"]) * x_price
            fee_amount += int(row["demand_y_fee"]) * y_price
            fee_amount += int(row["offer_y_fee"]) * y_price
            fee_amount += int(row["demand_x_fee"]) * x_price
    
    print(f"total swapped amount: {swap_amount}")
    print(f"total fees paid: {fee_amount}")
