package ent

// 需求的软删除依赖这两个特性：
// intercept 提供查询拦截器，统一过滤 deleted_at，免得每处查询手写条件；
// schema/snapshot 让 runtime 读取内置快照而不是反向 import schema 包，
// 否则软删除 mixin 引用 ent 生成包会形成 import 循环
//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --feature intercept,schema/snapshot ./schema
