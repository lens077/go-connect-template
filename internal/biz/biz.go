package biz

import "go.uber.org/fx"

// +co:anchor 是 co-cli 插入新资源 provider 的位置标记;+co:example 标的是
// 随模板附带的示例资源,co new 默认会连同它的文件一起删掉。两者对编译器都只是注释。

var Module = fx.Module("biz",
	fx.Provide(
		NewSearchUseCase, // +co:example
		// +co:anchor biz-providers
	),
)
