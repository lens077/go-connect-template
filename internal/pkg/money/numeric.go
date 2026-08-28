package money

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"
)

var errInvalidCents = errors.New("money value must use at most two decimal places")

// NumericToCents converts an exact PostgreSQL NUMERIC amount to integer cents.
func NumericToCents(n pgtype.Numeric) (int64, error) {
	if !n.Valid || n.Int == nil {
		return 0, errors.New("numeric value is NULL")
	}

	value := new(big.Int).Set(n.Int)
	scale := n.Exp + 2
	if scale >= 0 {
		value.Mul(value, pow10(scale))
	} else {
		divisor := pow10(-scale)
		quotient, remainder := new(big.Int), new(big.Int)
		quotient.QuoRem(value, divisor, remainder)
		if remainder.Sign() != 0 {
			return 0, errInvalidCents
		}
		value = quotient
	}

	if !value.IsInt64() {
		return 0, errors.New("numeric cents overflow int64")
	}
	return value.Int64(), nil
}

// CentsToNumeric converts integer cents to an exact PostgreSQL NUMERIC amount.
func CentsToNumeric(cents int64) (pgtype.Numeric, error) {
	if cents < 0 {
		return pgtype.Numeric{}, fmt.Errorf("cents must be non-negative: %d", cents)
	}
	return pgtype.Numeric{Int: big.NewInt(cents), Exp: -2, Valid: true}, nil
}

// NumericToFloat keeps the generic scaffold API compatible with existing resources.
// New financial APIs should prefer NumericToCents for two-decimal money values.
func NumericToFloat(n pgtype.Numeric) (float64, error) {
	value, err := n.Float64Value()
	if err != nil {
		return 0, fmt.Errorf("convert numeric to float64: %w", err)
	}
	if !value.Valid {
		return 0, errors.New("numeric value is NULL")
	}
	return value.Float64, nil
}

// Float64ToNumeric remains for neutral examples that accept decimal values.
// Financial APIs should expose integer cents and call CentsToNumeric instead.
func Float64ToNumeric(value float64) (pgtype.Numeric, error) {
	var numeric pgtype.Numeric
	if err := numeric.Scan(fmt.Sprintf("%v", value)); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("convert float64 to numeric: %w", err)
	}
	return numeric, nil
}

func pow10(exponent int32) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
}
