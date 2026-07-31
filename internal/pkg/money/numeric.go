// Package money 处理金额在 pgtype.Numeric 与 Go 原生类型之间的转换。
//
// 数据库里金额一律用 NUMERIC 存(定点十进制),不用 float8:float 存不下
// 0.1 这类十进制小数,累加几次就会出现分位误差。float64 只在出口(向上层
// 返回、序列化成 JSON)才出现,不参与运算。
package money

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// NumericToFloat 把数据库读出的 NUMERIC 转成 float64。
// NULL 与超出 float64 表示范围的值都返回错误,不静默给 0 —— 金额场景下
// 「0 元」和「没有值」是两回事,混同会直接算错账。
func NumericToFloat(n pgtype.Numeric) (float64, error) {
	if !n.Valid {
		return 0, fmt.Errorf("numeric value is NULL")
	}

	floatValue, err := n.Float64Value()
	if err != nil {
		return 0, fmt.Errorf("convert numeric to float64 failed: %w", err)
	}

	if !floatValue.Valid {
		return 0, fmt.Errorf("numeric cannot be represented as float64")
	}

	return floatValue.Float64, nil
}

// Float64ToNumeric 把 float64 转成可写库的 pgtype.Numeric。
//
// 走字符串中转而非直接赋值:Numeric.Scan 解析十进制文本,能拿到精确的
// 定点表示;若直接按二进制浮点构造,float64 本身的表示误差会被原样写进库。
func Float64ToNumeric(value float64) (pgtype.Numeric, error) {
	str := fmt.Sprintf("%v", value)

	var numeric pgtype.Numeric
	if err := numeric.Scan(str); err != nil {
		return pgtype.Numeric{}, fmt.Errorf("failed to convert float64 to pgtype.Numeric: %w", err)
	}

	return numeric, nil
}
