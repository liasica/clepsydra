package service

import (
	"fmt"
	"hash/fnv"
	"math"
)

// tagColor 按标签名生成十六进制颜色：FNV-1a hash 取色相，固定饱和度 65%、亮度 50%
// 仅在创建标签时调用一次并固化存库，同名必得同色；改名不重算，历史标签颜色不随算法调整变化
func tagColor(name string) string {
	h := fnv.New32a()
	h.Write([]byte(name))
	hue := float64(h.Sum32() % 360)

	r, g, b := hslToRGB(hue, 0.65, 0.50)

	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// hslToRGB HSL 转 RGB，h 取值 [0,360)，s / l 取值 [0,1]
func hslToRGB(h, s, l float64) (uint8, uint8, uint8) {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2

	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}

	return uint8(math.Round((r + m) * 255)),
		uint8(math.Round((g + m) * 255)),
		uint8(math.Round((b + m) * 255))
}
