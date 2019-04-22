package gecko

import "context"

type SessionHandler func(session session)

// 是一个二级Channel的事件处理器
type Dispatcher struct {
	startChan    chan session
	endChan      chan session
	startHandler SessionHandler
	endHandler   SessionHandler
}

func NewDispatcher(capacity int) *Dispatcher {
	return &Dispatcher{
		startChan: make(chan session, capacity),
		endChan:   make(chan session, capacity),
	}
}

func (d *Dispatcher) SetStartHandler(handler SessionHandler) {
	d.startHandler = handler
}

func (d *Dispatcher) SetEndHandler(handler SessionHandler) {
	d.endHandler = handler
}

func (d *Dispatcher) StartC() chan<- session {
	return d.startChan
}

func (d *Dispatcher) EndC() chan<- session {
	return d.endChan
}

func (d *Dispatcher) Serve(shutdown context.Context) {
	var start <-chan session = d.startChan
	var end <-chan session = d.endChan
	for {
		select {
		case <-shutdown.Done():
			return

		case v := <-start:
			go d.startHandler(v)

		case v := <-end:
			go d.endHandler(v)

		}
	}
}
