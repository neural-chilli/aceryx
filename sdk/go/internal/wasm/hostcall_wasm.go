//go:build wasm

package wasm

//go:wasmimport aceryx host_http_request
func hostHTTPRequest(inputPtr, inputLen uint32) uint64

//go:wasmimport aceryx host_call_connector
func hostCallConnector(inputPtr, inputLen uint32) uint64

//go:wasmimport aceryx host_case_get
func hostCaseGet(inputPtr, inputLen uint32) uint64

//go:wasmimport aceryx host_case_set
func hostCaseSet(inputPtr, inputLen uint32) uint64

//go:wasmimport aceryx host_vault_read
func hostVaultRead(inputPtr, inputLen uint32) uint64

//go:wasmimport aceryx host_vault_write
func hostVaultWrite(inputPtr, inputLen uint32) uint64

//go:wasmimport aceryx host_secret_get
func hostSecretGet(inputPtr, inputLen uint32) uint64

//go:wasmimport aceryx host_log
func hostLog(inputPtr, inputLen uint32) uint64

//go:wasmimport aceryx host_config_get
func hostConfigGet(inputPtr, inputLen uint32) uint64

//go:wasmimport aceryx host_create_case
func hostCreateCase(inputPtr, inputLen uint32) uint64

//go:wasmimport aceryx host_emit_event
func hostEmitEvent(inputPtr, inputLen uint32) uint64

//go:wasmimport aceryx host_queue_consume
func hostQueueConsume(inputPtr, inputLen uint32) uint64

//go:wasmimport aceryx host_queue_ack
func hostQueueAck(inputPtr, inputLen uint32) uint64

//go:wasmimport aceryx host_file_watch
func hostFileWatch(inputPtr, inputLen uint32) uint64
