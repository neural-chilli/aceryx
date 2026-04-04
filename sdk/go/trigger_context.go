package sdk

type triggerContextImpl struct {
	*contextImpl
}

func NewTriggerContext() *triggerContextImpl {
	return &triggerContextImpl{contextImpl: newContextWithBridge(newWASMHostBridge())}
}

func newTriggerContextWithBridge(host hostBridge) *triggerContextImpl {
	return &triggerContextImpl{contextImpl: newContextWithBridge(host)}
}

func (c *triggerContextImpl) CreateCase(caseType string, data interface{}) (string, error) {
	return c.host.CreateCase(caseType, data)
}

func (c *triggerContextImpl) EmitEvent(eventType string, payload interface{}) error {
	return c.host.EmitEvent(eventType, payload)
}

func (c *triggerContextImpl) QueueConsume(driverID, topic string) (message []byte, metadata map[string]string, messageID string, err error) {
	return c.host.QueueConsume(driverID, topic)
}

func (c *triggerContextImpl) QueueAck(driverID, messageID string) error {
	return c.host.QueueAck(driverID, messageID)
}

func (c *triggerContextImpl) FileWatch(driverID, path string) (event FileEvent, err error) {
	return c.host.FileWatch(driverID, path)
}

func (c *triggerContextImpl) PollHTTP(url string, headers map[string]string, intervalMS int) (Response, error) {
	return c.pollHTTP(url, headers, intervalMS)
}

var _ TriggerContext = (*triggerContextImpl)(nil)
