package bundles

import (
	"github.com/parkingwang/go-conf"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/tidwall/evio"
	"github.com/yoojia/go-gecko"
	"time"
)

//
// Author: 陈哈哈 chenyongjia@parkingwang.com, yoojiachen@gmail.com
//

// 工厂函数
func NetworkServerTriggerFactory() (string, func() interface{}) {
	return "NetworkServerTrigger", func() interface{} {
		return NewNetworkServerTrigger()
	}
}

func NewNetworkServerTrigger() *NetworkServerTrigger {
	return &NetworkServerTrigger{
		AbcTrigger: new(gecko.AbcTrigger),
	}
}

// 使用Evio框架的事件式网络服务器触发器
type NetworkServerTrigger struct {
	*gecko.AbcTrigger
	// 数据序列化与反序列化
	decoder gecko.Decoder
	encoder gecko.Encoder
	//
	ioEvents      evio.Events
	bindAddrGroup []string
	shutdownReady bool
	shutdown      chan struct{}
	//
	incomeFilter func(income map[string]interface{}, invoker gecko.Invoker)
}

func (ns *NetworkServerTrigger) SetDecoder(decoder gecko.Decoder) {
	ns.decoder = decoder
}

func (ns *NetworkServerTrigger) SetEncoder(encoder gecko.Encoder) {
	ns.encoder = encoder
}

func (ns *NetworkServerTrigger) SetIncomeFilter(hook func(income map[string]interface{}, invoker gecko.Invoker)) {
	ns.incomeFilter = hook
}

func (ns *NetworkServerTrigger) OnInit(args map[string]interface{}, scoped gecko.Context) {
	ns.shutdownReady = false
	ns.shutdown = make(chan struct{}, 1)
	if ns.decoder == nil {
		ns.SetDecoder(gecko.JSONDefaultDecoder)
		ns.withTag(log.Debug).Msg("使用默认JSONDecoder")
	}
	if ns.encoder == nil {
		ns.SetEncoder(gecko.JSONDefaultEncoder)
		ns.withTag(log.Debug).Msg("使用默认JSONEncoder")
	}
	config := conf.MapToMap(args)
	if group, err := config.MustStringArray("bindAddrGroup"); nil != err || len(group) == 0 {
		ns.withTag(log.Panic).Err(err).Msg("配置字段[bindAddrGroup]必须是个字符串数组")
	} else {
		ns.bindAddrGroup = group
	}
	ns.withTag(log.Info).Msg("NetworkTrigger服务初始化...")
}

func (ns *NetworkServerTrigger) OnStart(ctx gecko.Context, invoker gecko.Invoker) {
	ns.withTag(log.Info).Msgf("Network服务启动，绑定地址： %s", ns.bindAddrGroup)
	// Events
	ns.ioEvents.Data = func(conn evio.Conn, in []byte) (out []byte, action evio.Action) {
		// 使用Invoker调度内部系统处理，完成后返回给客户端
		resp := make(map[string]interface{})
		if json, deErr := ns.decoder(in); nil == deErr {
			if ns.incomeFilter != nil {
				ns.incomeFilter(json, invoker)
			}
			// 没有解析数据，忽略
			if nil == json || 0 == len(json) {
				resp = _makeError("NopPayload", "无效事件，忽略")
			} else {
				resp = invoker.Execute(gecko.NewTriggerEvent(ns.GetTopic(), json))
			}
		} else {
			resp = _makeError("DecodeError", deErr.Error())
			ns.withTag(log.Error).Err(deErr).Msg("服务器接收到无法解析的数据：" + conn.RemoteAddr().String())
		}
		// 编码返回
		if bytes, enErr := ns.encoder(resp); nil == enErr {
			out = bytes
		} else {
			out = []byte("{\"error\": \"EncodeError\"}")
			ns.withTag(log.Error).Err(enErr).Msg("服务器无法序列化的数据")
		}
		return
	}
	// 定时检查服务关闭
	// FIXME 并不能很好地解决如何平滑关闭Evio服务器的问题
	ns.ioEvents.Tick = func() (time.Duration, evio.Action) {
		if ns.shutdownReady {
			return time.Nanosecond, evio.Shutdown
		} else {
			return time.Millisecond * 500, evio.None
		}
	}
	// Serve
	go func() {
		defer func() {
			ns.withTag(log.Debug).Msg("Network服务停止")
			ns.shutdown <- struct{}{}
		}()
		// 绑定服务
		if err := evio.Serve(ns.ioEvents, ns.bindAddrGroup...); nil != err {
			ns.withTag(log.Error).Err(err).Msg("Network服务异常")
		}
	}()
}

func (ns *NetworkServerTrigger) OnStop(ctx gecko.Context, invoker gecko.Invoker) {
	ns.shutdownReady = true
	ns.withTag(log.Info).Msg("Network服务准备关闭...")
	<-ns.shutdown
}

func (ns *NetworkServerTrigger) withTag(fun func() *zerolog.Event) *zerolog.Event {
	return fun().Str("tag", "NetworkServerTrigger")
}

func _makeError(err string, msg string) map[string]interface{} {
	json := make(map[string]interface{}, 2)
	json["error"] = err
	json["message"] = msg
	return json
}
