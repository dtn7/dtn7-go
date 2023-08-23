package dummy_cla

type DummyListener struct {
	address string
	running bool
}

func NewDummyListener(address string) *DummyListener {
	listener := DummyListener{address: address, running: false}
	return &listener
}

func (listener *DummyListener) Address() string {
	return listener.address
}

func (listener *DummyListener) Start() error {
	listener.running = true
	return nil
}

func (listener *DummyListener) Running() bool {
	return listener.running
}

func (listener *DummyListener) Close() error {
	return nil
}
