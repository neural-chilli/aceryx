package hostfns

import "fmt"

func QueueConsume(_ string, _ []byte, _ string) ([]byte, map[string]string, string, error) {
	return nil, nil, "", fmt.Errorf("not implemented")
}

func QueueAck(_ string, _ string) error {
	return fmt.Errorf("not implemented")
}
