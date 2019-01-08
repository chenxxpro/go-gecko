package gecko

//
// Author: 陈哈哈 chenyongjia@parkingwang.com, yoojiachen@gmail.com
//

// 触发器事件
type Income struct {
	topic string
	data  map[string]interface{}
}

// 处理结果回调接口
type TriggerCallback func(topic string, data map[string]interface{})

// 用来发起请求，并输出结果
type TriggerInvoker func(income *Income, callback TriggerCallback)

// Trigger是一个负责接收前端事件，并调用 {@link ContextInvoker} 方法函数来向系统内部发起触发事件通知；
// 内部系统处理完成后，将回调完成函数，返回输出
type Trigger interface {
	NeedInitialize

	// 启动
	OnStart(scoped GeckoScoped, invoker TriggerInvoker)

	// 停止
	OnStop(scoped GeckoScoped, invoker TriggerInvoker)
}
