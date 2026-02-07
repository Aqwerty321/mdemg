package plugins

// InjectModuleForTest allows injecting a mock module for testing purposes.
// This function should only be used in test code.
func (m *Manager) InjectModuleForTest(info *ModuleInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modules[info.Manifest.ID] = &moduleInstance{
		info:           info,
		stopHealthLoop: make(chan struct{}),
	}
}

// ClearModulesForTest removes all modules for testing.
// This function should only be used in test code.
func (m *Manager) ClearModulesForTest() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modules = make(map[string]*moduleInstance)
}
