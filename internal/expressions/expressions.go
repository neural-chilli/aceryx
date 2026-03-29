package expressions

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

var (
	ErrExpressionTooLarge = errors.New("expressions: expression exceeds 4KB limit")
	ErrEvaluationTimeout  = errors.New("expressions: evaluation timed out")
)

const (
	maxExpressionSize = 4 * 1024
	defaultTimeout    = 100 * time.Millisecond
)

type runtimeSlot struct {
	vm *goja.Runtime
}

// Evaluator evaluates sandboxed JavaScript expressions with pooled runtimes.
type Evaluator struct {
	pool    sync.Pool
	timeout time.Duration
}

func NewEvaluator() *Evaluator {
	ev := &Evaluator{timeout: defaultTimeout}
	ev.pool.New = func() interface{} {
		vm := goja.New()
		_ = vm.Set("addDays", func(dateValue string, days int) string {
			t, err := time.Parse(time.RFC3339, dateValue)
			if err != nil {
				return dateValue
			}
			return t.AddDate(0, 0, days).Format(time.RFC3339)
		})
		_ = vm.Set("lower", strings.ToLower)
		_ = vm.Set("upper", strings.ToUpper)
		_ = vm.Set("contains", func(arr interface{}, needle interface{}) bool {
			rv := reflect.ValueOf(arr)
			if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
				return false
			}
			for i := 0; i < rv.Len(); i++ {
				if reflect.DeepEqual(rv.Index(i).Interface(), needle) {
					return true
				}
			}
			return false
		})
		_ = vm.Set("lenOf", func(v interface{}) int {
			rv := reflect.ValueOf(v)
			switch rv.Kind() {
			case reflect.Array, reflect.Slice, reflect.Map, reflect.String:
				return rv.Len()
			default:
				return 0
			}
		})
		return &runtimeSlot{vm: vm}
	}
	return ev
}

func (ev *Evaluator) Evaluate(expr string, context map[string]interface{}) (interface{}, error) {
	if len(expr) > maxExpressionSize {
		return nil, ErrExpressionTooLarge
	}

	slot := ev.pool.Get().(*runtimeSlot)
	defer ev.pool.Put(slot)

	vm := slot.vm
	_ = vm.Set("context", context)
	_ = vm.Set("caseData", context["case"])
	keys := make([]string, 0, len(context))
	for k, v := range context {
		_ = vm.Set(k, v)
		keys = append(keys, k)
	}

	defer func() {
		_ = vm.Set("context", nil)
		_ = vm.Set("caseData", nil)
		for _, k := range keys {
			_ = vm.Set(k, nil)
		}
		vm.ClearInterrupt()
	}()

	timer := time.AfterFunc(ev.timeout, func() {
		vm.Interrupt(ErrEvaluationTimeout)
	})
	defer timer.Stop()

	script := "(function(){ return (" + normalizeExpression(expr) + "); })()"
	value, err := vm.RunString(script)
	if err != nil {
		if errors.Is(err, ErrEvaluationTimeout) {
			return nil, ErrEvaluationTimeout
		}
		var interrupted *goja.InterruptedError
		if errors.As(err, &interrupted) {
			if interrupted.Value() == ErrEvaluationTimeout {
				return nil, ErrEvaluationTimeout
			}
		}
		return nil, fmt.Errorf("evaluate expression: %w", err)
	}
	return value.Export(), nil
}

var casePrefixRegex = regexp.MustCompile(`\bcase\.`)

func normalizeExpression(expr string) string {
	return casePrefixRegex.ReplaceAllString(expr, "caseData.")
}

func (ev *Evaluator) EvaluateBool(expr string, context map[string]interface{}) (bool, error) {
	v, err := ev.Evaluate(expr, context)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if ok {
		return b, nil
	}
	return false, nil
}
