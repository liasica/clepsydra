package service

import (
	"regexp"
	"testing"
)

// TestTagColor 颜色生成的确定性与格式
func TestTagColor(t *testing.T) {
	c1 := tagColor("优化")
	c2 := tagColor("优化")
	if c1 != c2 {
		t.Errorf("同名应得同色: %s != %s", c1, c2)
	}

	if !regexp.MustCompile(`^#[0-9a-f]{6}$`).MatchString(c1) {
		t.Errorf("颜色格式应为小写十六进制: %s", c1)
	}

	// 抽样几个常见标签名，不应全部撞色
	if tagColor("新功能") == tagColor("缺陷修复") && tagColor("缺陷修复") == tagColor("重构") {
		t.Error("多个不同名称全部同色，hash 可能失效")
	}
}
