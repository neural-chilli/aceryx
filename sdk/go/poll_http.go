package sdk

import "time"

func (c *triggerContextImpl) pollHTTP(url string, headers map[string]string, intervalMS int) (Response, error) {
	resp, err := c.HTTP(Request{
		Method:  "GET",
		URL:     url,
		Headers: headers,
	})
	if err != nil {
		return resp, err
	}
	if intervalMS > 0 {
		time.Sleep(time.Duration(intervalMS) * time.Millisecond)
	}
	return resp, nil
}
