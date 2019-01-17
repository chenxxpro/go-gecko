package gecko

//
// Author: 陈哈哈 chenyongjia@parkingwang.com, yoojiachen@gmail.com

// Bundle 是一个具有生命周期管理的接口
type Bundle interface {
	Initialize
	// 启动
	OnStart(ctx Context)
	// 停止
	OnStop(ctx Context)
}

// 组件创建工厂函数
type BundleFactory func() interface{}

// 编码解码工厂函数
type CodecFactory func() interface{}

// Plugin 用于隔离其它类型与 Bundle 的类型。
type Plugin interface {
	Bundle
}

// 生命周期Hook
type HookFunc func(pipeline *Pipeline)