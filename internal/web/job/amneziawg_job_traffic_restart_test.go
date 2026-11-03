package job

type transitionEmbeddedManager struct {
	removed []int
	started []int
}

func (m *transitionEmbeddedManager) Remove(id int) { m.removed = append(m.removed, id) }

type transitionKernelManager struct {
	id          int
	running     bool
	failRemoval bool
}

func (m *transitionKernelManager) Running(id int) bool { return id == m.id && m.running }
