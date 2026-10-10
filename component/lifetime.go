package component

type owned interface {
	ownerShutdown()
	setRelease(release func())
}

type lifetime struct {
	release  func()
	detached bool
}

func (l *lifetime) setRelease(fn func()) { l.release = fn }

func (l *lifetime) unown() {
	if l.release != nil {
		l.release()
		l.release = nil
	}
}

func (l *lifetime) detach(loop Loop, shutdown func()) {
	if l.detached {
		return
	}
	l.detached = true
	l.unown()
	l.release = loop.OnStop(shutdown)
}
