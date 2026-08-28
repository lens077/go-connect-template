package money

import (
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

func TestNumericToCents(t *testing.T) {
	tests := []struct {
		name    string
		numeric pgtype.Numeric
		want    int64
		wantErr bool
	}{
		{name: "whole yuan", numeric: pgtype.Numeric{Int: big.NewInt(12), Exp: 0, Valid: true}, want: 1200},
		{name: "one cent", numeric: pgtype.Numeric{Int: big.NewInt(1), Exp: -2, Valid: true}, want: 1},
		{name: "trailing fractional zero", numeric: pgtype.Numeric{Int: big.NewInt(1230), Exp: -3, Valid: true}, want: 123},
		{name: "fraction below cent", numeric: pgtype.Numeric{Int: big.NewInt(1234), Exp: -3, Valid: true}, wantErr: true},
		{name: "null", numeric: pgtype.Numeric{}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NumericToCents(tt.numeric)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCentsToNumericRoundTrip(t *testing.T) {
	for _, cents := range []int64{0, 1, 123, 9_999_999_999} {
		numeric, err := CentsToNumeric(cents)
		require.NoError(t, err)
		got, err := NumericToCents(numeric)
		require.NoError(t, err)
		require.Equal(t, cents, got)
	}

	_, err := CentsToNumeric(-1)
	require.Error(t, err)
}
