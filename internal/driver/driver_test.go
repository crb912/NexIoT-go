package driver

type mockDriver struct{}

func (m *mockDriver) Initialize() error          { return nil }
func (m *mockDriver) Start() error               { return nil }
func (m *mockDriver) Stop() error                { return nil }
func (m *mockDriver) HandleReadCommands()        {}
func (m *mockDriver) HandleWriteCommands() error { return nil }
func (m *mockDriver) AddDevice() error           { return nil }
func (m *mockDriver) UpdateDevice() error        { return nil }
func (m *mockDriver) RemoveDevice() error        { return nil }
