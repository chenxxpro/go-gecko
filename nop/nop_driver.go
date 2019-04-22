package nop

import (
	"github.com/yoojia/go-gecko"
)

//
// Author: 陈哈哈 chenyongjia@parkingwang.com, yoojiachen@gmail.com
//

func NopDriverFactory() (string, gecko.Factory) {
	return "NopDriver", func() interface{} {
		return &NopDriver{
			AbcDriver: gecko.NewAbcDriver(),
		}
	}
}

// 触发UDP设备的模拟Driver
type NopDriver struct {
	*gecko.AbcDriver
	gecko.Initial
	gecko.LifeCycle
}

func (du *NopDriver) OnInit(config map[string]interface{}, ctx gecko.Context) {
	log.Debug("初始化...")
}

func (du *NopDriver) OnStart(ctx gecko.Context) {
	log.Debug("启动...")
}

func (du *NopDriver) OnStop(ctx gecko.Context) {
	log.Debug("停止...")
}

func (du *NopDriver) Handle(attrs gecko.Attributes, topic string, uuid string, in *gecko.MessagePacket, deliverer gecko.OutputDeliverer, ctx gecko.Context) error {
	//deliverer.Broadcast("127.0.0.1", gecko.JSONPacket{
	//	"A": "b",
	//})
	//if pipeline, ok := selector("udp"); !ok {
	//	return errors.New("无法查找到udp协议对应的Pipeline")
	//} else {
	//	// 通过Pipeline，向特定设备发送消息：
	//	//groupAddress := "目标设备的GroupAddress"
	//	//privateAddress := "目标设备的PrivateAddress"
	//	//resp, err := pipeline.ExecuteDevice(groupAddress, privateAddress, gecko.NewFramePacket([]byte{0}))
	//
	//}

	return nil
}
