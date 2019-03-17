package gecko

import (
	"context"
	"github.com/parkingwang/go-conf"
	"github.com/pkg/errors"
	"github.com/yoojia/go-gecko/structs"
	"github.com/yoojia/go-gecko/utils"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"
)

////

// 默认组件生命周期超时时间：3秒
const DefaultLifeCycleTimeout = time.Second * 3

var gSharedPipeline = &Pipeline{
	Register: prepare(),
}
var gPrepareEnv = new(sync.Once)

// 全局Pipeline对象
func SharedPipeline() *Pipeline {
	gPrepareEnv.Do(func() {
		gSharedPipeline.prepareEnv()
	})
	return gSharedPipeline
}

// Pipeline管理内部组件，处理事件。
type Pipeline struct {
	*Register
	context Context
	// 事件派发
	dispatcher *Dispatcher
	// Pipeline关闭的信号控制
	shutdownCtx  context.Context
	shutdownFunc context.CancelFunc
}

// 初始化Pipeline
func (p *Pipeline) Init(config *cfg.Config) {
	zlog := ZapSugarLogger
	ctx := p.newGeckoContext(config)
	p.context = ctx
	gecko := p.context.gecko()
	capacity := gecko.GetInt64OrDefault("eventsCapacity", 8)
	zlog.Infof("事件通道容量： %d", capacity)
	p.dispatcher = NewDispatcher(int(capacity))
	p.dispatcher.SetStartHandler(p.handleInterceptor)
	p.dispatcher.SetEndHandler(p.handleDriver)

	go p.dispatcher.Serve(p.shutdownCtx)

	// 初始化组件：根据配置文件指定项目
	initFn := func(it NeedInit, args *cfg.Config) {
		it.OnInit(args, p.context)
	}
	// 使用结构化的参数来初始化
	initStructFn := func(it NeedStructInit, args *cfg.Config) {
		config := it.GetConfigStruct()
		decoder, err := structs.NewDecoder(&structs.DecoderConfig{
			TagName: "toml",
			Result:  config,
		})
		if nil != err {
			zlog.Panic("无法创建Map2Struct解码器", err)
		}
		if err := decoder.Decode(args.RefMap()); nil != err {
			zlog.Panic("Map2Struct解码出错", err)
		}
		it.OnInit(config, p.context)
	}

	if ctx.cfgPlugins.IsEmpty() {
		zlog.Warn("警告：未配置任何[Plugin]组件")
	} else {
		p.register(ctx.cfgPlugins, initFn, initStructFn)
	}
	if ctx.cfgOutputs.IsEmpty() {
		zlog.Fatal("严重：未配置任何[OutputDevice]组件")
	} else {
		p.register(ctx.cfgOutputs, initFn, initStructFn)
	}
	if ctx.cfgInterceptors.IsEmpty() {
		zlog.Warn("警告：未配置任何[Interceptor]组件")
	} else {
		p.register(ctx.cfgInterceptors, initFn, initStructFn)
	}
	if ctx.cfgDrivers.IsEmpty() {
		zlog.Warn("警告：未配置任何[Driver]组件")
	} else {
		p.register(ctx.cfgDrivers, initFn, initStructFn)
	}
	if ctx.cfgInputs.IsEmpty() {
		zlog.Fatal("严重：未配置任何[InputDevice]组件")
	} else {
		p.register(ctx.cfgInputs, initFn, initStructFn)
	}
	if !ctx.cfgLogics.IsEmpty() {
		p.register(ctx.cfgLogics, initFn, initStructFn)
	} else {
		zlog.Warn("警告：未配置任何[LogicDevice]组件")
	}
	// show
	p.showBundles()
}

// 启动Pipeline
func (p *Pipeline) Start() {
	zlog := ZapSugarLogger
	zlog.Info("Pipeline启动...")
	// Hook first
	utils.ForEach(p.startBeforeHooks, func(it interface{}) {
		it.(HookFunc)(p)
	})
	defer func() {
		utils.ForEach(p.startAfterHooks, func(it interface{}) {
			it.(HookFunc)(p)
		})
		zlog.Info("Pipeline启动...OK")
	}()

	startFn := func(component interface{}) {
		if stoppable, ok := component.(LifeCycle); ok {
			p.context.CheckTimeout(utils.GetClassName(component)+".Start", DefaultLifeCycleTimeout, func() {
				stoppable.OnStart(p.context)
			})
		}
	}

	// Plugins
	utils.ForEach(p.plugins, startFn)
	// Outputs
	utils.ForEach(p.outputs, startFn)
	// Drivers
	utils.ForEach(p.drivers, startFn)
	// Inputs
	utils.ForEach(p.inputs, startFn)
	// Then, Serve inputs
	utils.ForEach(p.inputs, func(it interface{}) {
		input := it.(InputDevice)
		deliverer := p.newInputDeliverer(input)
		go func() {
			uuid := input.GetUuid()
			defer zlog.Debugf("InputDevice已经停止：%s", uuid)
			err := input.Serve(p.context, deliverer)
			if nil != err {
				zlog.Errorw("InputDevice服务运行错误",
					"uuid", uuid,
					"error", err,
					"class", utils.GetClassName(input))
			}
		}()
	})
}

// 停止Pipeline
func (p *Pipeline) Stop() {
	zlog := ZapSugarLogger
	zlog.Info("Pipeline停止...")
	// Hook first
	utils.ForEach(p.stopBeforeHooks, func(it interface{}) {
		it.(HookFunc)(p)
	})
	defer func() {
		utils.ForEach(p.stopAfterHooks, func(it interface{}) {
			it.(HookFunc)(p)
		})
		// 最终发起关闭信息
		p.shutdownFunc()
		zlog.Info("Pipeline停止...OK")
	}()

	stopFn := func(component interface{}) {
		if stoppable, ok := component.(LifeCycle); ok {
			p.context.CheckTimeout(utils.GetClassName(component)+".Stop", DefaultLifeCycleTimeout, func() {
				stoppable.OnStop(p.context)
			})
		}
	}
	// Inputs
	utils.ForEach(p.inputs, stopFn)
	// Drivers
	utils.ForEach(p.drivers, stopFn)
	// Outputs
	utils.ForEach(p.outputs, stopFn)
	// Plugins
	utils.ForEach(p.plugins, stopFn)
}

// 等待系统终止信息
func (p *Pipeline) AwaitTermination() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	ZapSugarLogger.Info("接收到系统停止信号")
}

func (p *Pipeline) init() {

}

// 准备运行环境，初始化相关组件
func (p *Pipeline) prepareEnv() {
	p.shutdownCtx, p.shutdownFunc = context.WithCancel(context.Background())
}

// 创建InputDeliverer函数
// InputDeliverer函数对于InputDevice对象是一个系统内部数据传输流程的代理函数。
// 每个Deliver请求，都会向系统发起请求，并获取系统处理结果响应数据。也意味着，InputDevice发起的每个请求
// 都会执行 Decode -> Deliver(GeckoKernelFlow) -> Encode 流程。
func (p *Pipeline) newInputDeliverer(masterInput InputDevice) InputDeliverer {
	return InputDeliverer(func(topic string, rawFrame FramePacket) (FramePacket, error) {
		// 从Input设备中读取Decode数据
		masterUuid := masterInput.GetUuid()
		inputFrame := rawFrame.Data()
		if nil == inputFrame {
			return nil, errors.New("Input设备发起Deliver请求必须携带参数数据")
		}
		inputJSON, err := masterInput.GetDecoder()(inputFrame)
		if nil != err {
			return nil, errors.WithMessage(err, "Input设备Decode数据出错："+masterUuid)
		}
		outputCh := make(chan JSONPacket, 1)
		attributes := make(map[string]interface{})
		attributes["@InputDevice.Type"] = utils.GetClassName(masterInput)
		attributes["@InputDevice.Name"] = masterInput.GetName()

		toSendUuid := masterUuid
		toSendTopic := topic
		toSendData := inputJSON

		var logic LogicDevice = nil
		// 查找符合条件的逻辑设备，并转换数据
		for _, item := range masterInput.GetLogicDevices() {
			if item.CheckIfMatch(inputJSON) {
				logic = item
				attributes["@InputDevice.Logic.Type"] = utils.GetClassName(logic)
				attributes["@InputDevice.Logic.Name"] = logic.GetName()
				break
			}
		}
		if logic != nil {
			toSendUuid = logic.GetUuid()
			toSendTopic = logic.GetTopic()
			toSendData = logic.Transform(toSendData)
		}
		// 发送到Dispatcher调度处理
		p.dispatcher.StartC() <- &_EventSessionImpl{
			timestamp:  time.Now(),
			attributes: attributes,
			topic:      toSendTopic,
			uuid:       toSendUuid,
			inbound: &Message{
				Topic: toSendTopic,
				Data:  toSendData,
			},
			outbound: &Message{
				Topic: toSendTopic,
				Data:  make(map[string]interface{}),
			},
			outputChan: outputCh,
		}
		// 等待处理完成
		outputJSON := <-outputCh
		if nil == outputJSON {
			return nil, errors.New("Input设备发起Deliver请求必须返回结果数据")
		}
		if outputFrame, err := masterInput.GetEncoder()(outputJSON); nil != err {
			return nil, errors.WithMessage(err, "Input设备Encode数据出错："+masterUuid)
		} else {
			return NewFramePacket(outputFrame), nil
		}
	})
}

// 输出派发函数
// 根据Driver指定的目标输出设备地址，查找并处理数据包
func (p *Pipeline) deliverToOutput(uuid string, rawJSON JSONPacket) (JSONPacket, error) {
	if output, ok := p.uuidOutputs[uuid]; ok {
		inputFrame, encErr := output.GetEncoder().Encode(rawJSON)
		if nil != encErr {
			return nil, errors.WithMessage(encErr, "设备Encode数据出错: "+uuid)
		}
		retJSON, err := output.Process(inputFrame, p.context)
		if nil != err {
			return nil, errors.WithMessage(err, "Output设备处理出错: "+uuid)
		}
		if json, decErr := output.GetDecoder().Decode(retJSON); nil != decErr {
			return nil, errors.WithMessage(encErr, "设备Decode数据出错: "+uuid)
		} else {
			return json, nil
		}
	} else {
		return nil, errors.New("指定地址的Output设备不存在:" + uuid)
	}
}

// 处理拦截器过程
func (p *Pipeline) handleInterceptor(session EventSession) {
	zlog := ZapSugarLogger
	p.context.OnIfLogV(func() {
		zlog.Debugf("Interceptor调度处理，Topic: %s", session.Topic())
	})
	defer func() {
		p.checkRecover(recover(), "Interceptor-Goroutine内部错误")
	}()
	// 查找匹配的拦截器，按优先级排序并处理
	matches := make(InterceptorSlice, 0)
	for el := p.interceptors.Front(); el != nil; el = el.Next() {
		interceptor := el.Value.(Interceptor)
		match := anyTopicMatches(interceptor.GetTopicExpr(), session.Topic())
		p.context.OnIfLogV(func() {
			zlog.Debugf("拦截器调度： interceptor[%s], topic: %s, match: %s",
				utils.GetClassName(interceptor),
				session.Topic(),
				strconv.FormatBool(match))
		})
		if match {
			matches = append(matches, interceptor)
		}
	}
	sort.Sort(matches)
	// 按排序结果顺序执行
	for _, it := range matches {
		err := it.Handle(session, p.context)
		if err == nil {
			continue
		}
		if err == ErrInterceptorDropped {
			zlog.Debugf("拦截器中断事件： %s", err.Error())
			session.Outbound().AddDataField("error", "InterceptorDropped")
			// 终止，输出处理
			session.AddAttribute("拦截过程用时", session.Since())
			p.output(session)
			return
		} else {
			p.failFastLogger(err, "拦截器发生错误")
		}
	}
	// 继续
	session.AddAttribute("拦截过程用时", session.Since())
	p.dispatcher.EndC() <- session
}

// 处理驱动执行过程
func (p *Pipeline) handleDriver(session EventSession) {
	zlog := ZapSugarLogger
	p.context.OnIfLogV(func() {
		zlog.Debugf("Driver调度处理，Topic: %s", session.Topic())
	})
	defer func() {
		p.checkRecover(recover(), "Driver-Goroutine内部错误")
	}()

	// 查找匹配的用户驱动，并处理
	for el := p.drivers.Front(); el != nil; el = el.Next() {
		driver := el.Value.(Driver)
		match := anyTopicMatches(driver.GetTopicExpr(), session.Topic())
		p.context.OnIfLogV(func() {
			zlog.Debugf("用户驱动处理： driver[%s], topic: %s, match: %s",
				utils.GetClassName(driver),
				session.Topic(),
				strconv.FormatBool(match))
		})
		if match {
			err := driver.Handle(session, OutputDeliverer(p.deliverToOutput), p.context)
			if nil != err {
				p.failFastLogger(err, "用户驱动发生错误")
			}
		}
	}
	// 输出处理
	session.AddAttribute("驱动过程用时", session.Since())
	p.output(session)
}

func (p *Pipeline) output(event EventSession) {
	p.context.OnIfLogV(func() {
		zlog := ZapSugarLogger
		zlog.Debugf("Output调度处理，Topic: %s", event.Topic())
		event.Attributes().ForEach(func(k string, v interface{}) {
			zlog.Debugf("SessionAttr: %s = %v", k, v)
		})
	})
	defer func() {
		p.checkRecover(recover(), "Output-Goroutine内部错误")
	}()
	// 返回处理结果
	event.(*_EventSessionImpl).outputChan <- event.Outbound().Data
}

func (p *Pipeline) checkDefTimeout(msg string, act func(Context)) {
	p.context.CheckTimeout(msg, DefaultLifeCycleTimeout, func() {
		act(p.context)
	})
}

func (p *Pipeline) checkRecover(r interface{}, msg string) {
	if nil != r {
		zlog := ZapSugarLogger
		if err, ok := r.(error); ok {
			zlog.Errorw(msg, "error", err)
		}
		p.context.OnIfFailFast(func() {
			zlog.Fatal(r)
		})
	}
}

func (p *Pipeline) failFastLogger(err error, msg string) {
	zlog := ZapSugarLogger
	if p.context.IsFailFastEnabled() {
		zlog.Fatalw(msg, "error", err)
	} else {
		zlog.Errorw(msg, "error", err)
	}
}

func (p *Pipeline) newGeckoContext(config *cfg.Config) *_GeckoContext {
	return &_GeckoContext{
		cfgGeckos:       config.MustConfig("GECKO"),
		cfgGlobals:      config.MustConfig("GLOBALS"),
		cfgInterceptors: config.MustConfig("INTERCEPTORS"),
		cfgDrivers:      config.MustConfig("DRIVERS"),
		cfgOutputs:      config.MustConfig("OUTPUTS"),
		cfgInputs:       config.MustConfig("INPUTS"),
		cfgPlugins:      config.MustConfig("PLUGINS"),
		cfgLogics:       config.MustConfig("LOGICS"),
		scopedKV:        make(map[interface{}]interface{}),
		plugins:         p.plugins,
		interceptors:    p.interceptors,
		drivers:         p.drivers,
		outputs:         p.outputs,
		inputs:          p.inputs,
	}
}

func anyTopicMatches(expected []*TopicExpr, topic string) bool {
	for _, t := range expected {
		if t.matches(topic) {
			return true
		}
	}
	return false
}
