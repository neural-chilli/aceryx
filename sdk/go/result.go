package sdk

import "encoding/json"

func OK() Result {
	return Result{Status: "ok"}
}

func OKWithOutput(data interface{}) Result {
	raw, err := json.Marshal(data)
	if err != nil {
		return ErrorWithCode("SERIALIZATION_ERROR", err.Error())
	}
	return Result{Status: "ok", Output: raw}
}

func Error(msg string) Result {
	return Result{Status: "error", Error: msg}
}

func ErrorWithCode(code, msg string) Result {
	return Result{Status: "error", Error: msg, Code: code}
}
