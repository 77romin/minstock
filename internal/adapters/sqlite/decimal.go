package sqlite

import "github.com/shopspring/decimal"

func mustDecimal(v string) decimal.Decimal {
	d, err := decimal.NewFromString(v)
	if err != nil {
		return decimal.Zero
	}
	return d
}
