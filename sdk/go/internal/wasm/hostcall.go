package wasm

import (
	"encoding/json"
	"fmt"
	"unsafe"
)

func CallHTTP(input interface{}, out interface{}) error {
	return callJSON(hostHTTPRequest, input, out)
}

func CallConnector(input interface{}, out interface{}) error {
	return callJSON(hostCallConnector, input, out)
}

func CallCaseGet(input interface{}, out interface{}) error {
	return callJSON(hostCaseGet, input, out)
}

func CallCaseSet(input interface{}) error {
	return callJSON(hostCaseSet, input, nil)
}

func CallVaultRead(input interface{}, out interface{}) error {
	return callJSON(hostVaultRead, input, out)
}

func CallVaultWrite(input interface{}, out interface{}) error {
	return callJSON(hostVaultWrite, input, out)
}

func CallSecretGet(input interface{}, out interface{}) error {
	return callJSON(hostSecretGet, input, out)
}

func CallLog(input interface{}) error {
	return callJSON(hostLog, input, nil)
}

func CallConfigGet(input interface{}, out interface{}) error {
	return callJSON(hostConfigGet, input, out)
}

func CallCreateCase(input interface{}, out interface{}) error {
	return callJSON(hostCreateCase, input, out)
}

func CallEmitEvent(input interface{}) error {
	return callJSON(hostEmitEvent, input, nil)
}

func CallQueueConsume(input interface{}, out interface{}) error {
	return callJSON(hostQueueConsume, input, out)
}

func CallQueueAck(input interface{}) error {
	return callJSON(hostQueueAck, input, nil)
}

func CallFileWatch(input interface{}, out interface{}) error {
	return callJSON(hostFileWatch, input, out)
}

func callJSON(fn func(uint32, uint32) uint64, input interface{}, out interface{}) error {
	raw, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode host input: %w", err)
	}
	result := call(fn, raw)
	return decodeEnvelope(result, out)
}

func call(fn func(uint32, uint32) uint64, payload []byte) []byte {
	if len(payload) == 0 {
		packed := fn(0, 0)
		return readPackedResult(packed)
	}
	ptr := uint32(uintptr(unsafe.Pointer(&payload[0])))
	packed := fn(ptr, uint32(len(payload)))
	return readPackedResult(packed)
}

func readPackedResult(packed uint64) []byte {
	ptr := uint32(packed >> 32)
	length := uint32(packed & 0xffffffff)
	if ptr == 0 || length == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)
}
