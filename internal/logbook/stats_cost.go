package logbook

import (
	"database/sql"
	"math/big"
	"regexp"
)

var statsCostDecimal = regexp.MustCompile(`^(0|[1-9][0-9]*)([.][0-9]{1,30})?$`)

// statsCost accepts only frozen USD decimals. Missing adjustments never fall
// back to raw reference prices or today's prices and multipliers.
func statsCost(raw sql.NullString) *big.Rat {
	if !raw.Valid || len(raw.String) > 128 || !statsCostDecimal.MatchString(raw.String) {
		return nil
	}
	value, ok := new(big.Rat).SetString(raw.String)
	if !ok {
		return nil
	}
	return value
}

func (a *statsAccumulator) addCost(value *big.Rat) {
	if value == nil {
		return
	}
	a.costSamples++
	a.cost.Add(&a.cost, value)
}
