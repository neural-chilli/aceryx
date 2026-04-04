//go:build !wasm

package wasm

import "encoding/json"

func hostHTTPRequest(_, _ uint32) uint64   { return stubResult() }
func hostCallConnector(_, _ uint32) uint64 { return stubResult() }
func hostCaseGet(_, _ uint32) uint64       { return stubResult() }
func hostCaseSet(_, _ uint32) uint64       { return stubResult() }
func hostVaultRead(_, _ uint32) uint64     { return stubResult() }
func hostVaultWrite(_, _ uint32) uint64    { return stubResult() }
func hostSecretGet(_, _ uint32) uint64     { return stubResult() }
func hostLog(_, _ uint32) uint64           { return stubResult() }
func hostConfigGet(_, _ uint32) uint64     { return stubResult() }
func hostCreateCase(_, _ uint32) uint64    { return stubResult() }
func hostEmitEvent(_, _ uint32) uint64     { return stubResult() }
func hostQueueConsume(_, _ uint32) uint64  { return stubResult() }
func hostQueueAck(_, _ uint32) uint64      { return stubResult() }
func hostFileWatch(_, _ uint32) uint64     { return stubResult() }

func stubResult() uint64 {
	payload, _ := json.Marshal(hostEnvelope{
		OK:    false,
		Error: "host function unavailable outside wasm runtime",
	})
	if len(payload) == 0 {
		return 0
	}
	return uint64(uint32(0))<<32 | uint64(uint32(0))
}
