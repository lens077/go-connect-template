package main

import (
	"testing"

	"go.uber.org/fx"
)

// TestAppDependencyGraph 校验 fx 依赖图闭合:每个构造函数要的参数都有人 provide。
//
// fx.ValidateApp 只解析图、不执行任何构造函数,所以不需要配置文件、数据库或任何外部服务,
// 毫秒级完成。它守的是 go build / go vet 看不见的一类错误:
// 曾经 NewConnectOptions 要 *confv1.Observability 而没人 provide,编译通过、CI 全绿,
// 但生成出来的每一个服务在 fx 构建阶段就 start failed。
//
// 这个测试随生成物一起走,不带 +co: 标记:无论选了哪些 feature,图都必须闭合。
func TestAppDependencyGraph(t *testing.T) {
	if err := fx.ValidateApp(AppOptions("graph-test", "test", "v0")...); err != nil {
		t.Fatalf("fx dependency graph is broken:\n%v", err)
	}
}
