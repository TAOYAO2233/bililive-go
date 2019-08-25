package listeners

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/bluele/gcache"

	"github.com/hr3lxphr6j/bililive-go/src/configs"
	"github.com/hr3lxphr6j/bililive-go/src/instance"
	"github.com/hr3lxphr6j/bililive-go/src/interfaces"
	"github.com/hr3lxphr6j/bililive-go/src/lib/events"
	"github.com/hr3lxphr6j/bililive-go/src/live"
)

const (
	begin uint32 = iota
	pending
	running
	stopped
)

func NewListener(ctx context.Context, live live.Live) *Listener {
	inst := instance.GetInstance(ctx)
	return &Listener{
		Live:   live,
		status: false,
		config: inst.Config,
		cache:  inst.Cache,
		stop:   make(chan struct{}),
		ed:     inst.EventDispatcher.(events.Dispatcher),
		logger: inst.Logger,
		state:  begin,
	}
}

type Listener struct {
	Live   live.Live
	status bool

	config *configs.Config
	ed     events.Dispatcher
	logger *interfaces.Logger
	cache  gcache.Cache

	state uint32
	stop  chan struct{}
}

func (l *Listener) Start() error {
	if !atomic.CompareAndSwapUint32(&l.state, begin, pending) {
		return nil
	}
	defer atomic.CompareAndSwapUint32(&l.state, pending, running)

	l.ed.DispatchEvent(events.NewEvent(ListenStart, l.Live))
	l.refresh()
	go l.run()
	return nil
}

func (l *Listener) Close() {
	if !atomic.CompareAndSwapUint32(&l.state, running, stopped) {
		return
	}
	l.ed.DispatchEvent(events.NewEvent(ListenStop, l.Live))
	close(l.stop)
}

func (l *Listener) refresh() {
	info, err := l.Live.GetInfo()
	if err != nil {
		l.logger.
			WithError(err).
			WithField("url", l.Live.GetRawUrl()).
			Error("failed to load room info")
		return
	}
	l.cache.Set(l.Live, info)
	if info.Status == l.status {
		return
	}
	l.status = info.Status

	var (
		evtTyp  events.EventType
		logInfo string
		fields  = map[string]interface{}{
			"room": info.RoomName,
			"host": info.HostName,
		}
	)
	if l.status {
		l.Live.SetLastStartTime(time.Now())
		evtTyp = LiveStart
		logInfo = "Live Start"
	} else {
		evtTyp = LiveEnd
		logInfo = "Live end"
	}
	l.ed.DispatchEvent(events.NewEvent(evtTyp, l.Live))
	l.logger.WithFields(fields).Info(logInfo)
}

func (l *Listener) run() {
	ticker := time.NewTicker(time.Duration(l.config.Interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
			l.refresh()
		}
	}
}
